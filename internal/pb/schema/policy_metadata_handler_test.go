package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// pdpCall records one request received by the fake PDP
type pdpCall struct {
	Method string
	Path   string
	Query  string
	Body   string
}

// fakePDP is an in-process PDP that serves the policy metadata and access
// grants of one schema and records the requests it receives.
type fakePDP struct {
	schemaID string
	records  []policy.PolicyMetadataResponse
	grants   []policy.AccessGrant
	// writeStatus and writeBody override the response to write requests
	writeStatus int
	writeBody   string
	calls       []pdpCall
}

func (f *fakePDP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.calls = append(f.calls, pdpCall{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: string(body)})

	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/policy/grants" {
		query := r.URL.Query()
		grants := []policy.AccessGrant{}
		for _, grant := range f.grants {
			if (query.Get("schemaId") == "" || query.Get("schemaId") == grant.SchemaID) &&
				(query.Get("applicationId") == "" || query.Get("applicationId") == grant.ApplicationID) &&
				(query.Get("status") == "" || query.Get("status") == grant.Status) &&
				(query.Get("consentBypass") == "" || query.Get("consentBypass") == fmt.Sprint(grant.ConsentBypass)) {
				grants = append(grants, grant)
			}
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(policy.AccessGrantListResponse{Records: grants})
		return
	}

	if r.Method == http.MethodGet {
		records := []policy.PolicyMetadataResponse{}
		if r.URL.Query().Get("schemaId") == f.schemaID {
			records = f.records
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(policy.PolicyMetadataListResponse{Records: records})
		return
	}

	if f.writeStatus != 0 {
		w.WriteHeader(f.writeStatus)
		_, _ = w.Write([]byte(f.writeBody))
		return
	}

	switch r.Method {
	case http.MethodPatch:
		record := f.records[0]
		displayName := "Patched"
		record.DisplayName = &displayName
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(record)
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		for i, grant := range f.grants {
			if r.URL.Path == "/api/v1/policy/grants/"+grant.ID+"/revoke" {
				f.grants[i].Status = policy.GrantStatusRevoked
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(f.grants[i])
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// writeCalls returns the write requests received by the fake PDP
func (f *fakePDP) writeCalls() []pdpCall {
	var calls []pdpCall
	for _, call := range f.calls {
		if call.Method != http.MethodGet {
			calls = append(calls, call)
		}
	}
	return calls
}

type policyMetadataTestEnv struct {
	db       *gorm.DB
	handler  *Handler
	pdp      *fakePDP
	schemaID string
}

// newPolicyMetadataTestEnv creates a schema owned by a member other than the
// test users and a fake PDP holding one policy metadata record for it.
func newPolicyMetadataTestEnv(t *testing.T) *policyMetadataTestEnv {
	db := setupSQLiteTestDB(t)

	memberID := createTestMember(t, db, fmt.Sprintf("policy-metadata-%d@example.com", time.Now().UnixNano()))
	schemaID := createTestSchema(t, db, memberID)

	pdp := &fakePDP{
		schemaID: schemaID,
		records: []policy.PolicyMetadataResponse{
			{
				ID:          "pm-1",
				SchemaID:    schemaID,
				FieldName:   "person.name",
				Source:      policy.SourcePrimary,
				PolicyClass: policy.PolicyClassAllowlistConsent,
			},
		},
		grants: []policy.AccessGrant{
			{
				ID: "grant-1", SchemaID: schemaID, FieldName: "person.name", ApplicationID: "client-1",
				ExpiresAt: "2030-01-01T00:00:00Z", Status: policy.GrantStatusActive,
				CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z",
			},
			{
				// Revoked grants are not part of the allow-list.
				ID: "grant-0", SchemaID: schemaID, FieldName: "person.name", ApplicationID: "client-2",
				ExpiresAt: "2030-01-01T00:00:00Z", Status: policy.GrantStatusRevoked,
				CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z",
			},
			{
				// Consent bypass grants are not part of the allow-list either.
				ID: "grant-bypass", SchemaID: schemaID, FieldName: "person.name", ApplicationID: "client-3",
				ExpiresAt: "2030-01-01T00:00:00Z", Status: policy.GrantStatusActive, ConsentBypass: true,
				CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-02T00:00:00Z",
			},
		},
	}
	server := httptest.NewServer(pdp)
	t.Cleanup(server.Close)

	pdpService := policy.NewClient(server.URL)
	handler := NewHandler(NewService(db, pdpService))

	return &policyMetadataTestEnv{db: db, handler: handler, pdp: pdp, schemaID: schemaID}
}

// makeMemberOwner reassigns the schema to a new member and returns that member's
// test user. A fresh user is created per test so its organization unit is
// unique to this test database.
func (e *policyMetadataTestEnv) makeMemberOwner(t *testing.T) callertest.TestUser {
	owner := callertest.CreateCustomTestUser(fmt.Sprintf("owner-%d", time.Now().UnixNano()), "owner@test.com", fmt.Sprintf("owner-ou-%d", time.Now().UnixNano()), callertest.MemberScopes)
	m := member.Member{
		MemberID:    "mem_owner_" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:        "Owner Member",
		Email:       owner.Email,
		PhoneNumber: "1234567890",
		OuID:        owner.OuID,
	}
	require.NoError(t, e.db.Create(&m).Error)
	require.NoError(t, e.db.Model(&Schema{}).Where("schema_id = ?", e.schemaID).Update("member_id", m.MemberID).Error)

	return owner.WithMember(m.MemberID)
}

// newPolicyMetadataMux registers the policy metadata routes the same way cmd/pb/main.go does.
func newPolicyMetadataMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/schemas/{schemaId}/policy-metadata", h.ListSchemaPolicyMetadata)
	mux.HandleFunc("PATCH /api/v1/schemas/{schemaId}/policy-metadata/{id}", h.PatchSchemaPolicyMetadata)
	mux.HandleFunc("DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}", h.DeleteSchemaPolicyMetadata)
	mux.HandleFunc("DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}/allowlist/{applicationId}", h.RevokeSchemaPolicyAllowListEntry)
	return mux
}

func (e *policyMetadataTestEnv) serve(req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	newPolicyMetadataMux(e.handler).ServeHTTP(w, req)
	return w
}

func (e *policyMetadataTestEnv) url(suffix string) string {
	return fmt.Sprintf("/api/v1/schemas/%s/policy-metadata%s", e.schemaID, suffix)
}

func TestSchemaPolicyMetadataEndpoints_List(t *testing.T) {
	t.Run("Admin lists policy metadata of a schema", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodGet, env.url(""), nil))

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response struct {
			Items []policy.PolicyMetadataResponse `json:"items"`
			Count int                             `json:"count"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, 1, response.Count)
		assert.Equal(t, "pm-1", response.Items[0].ID)
		assert.Equal(t, policy.AllowList{
			"client-1": {
				ExpiresAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			},
		}, response.Items[0].AllowList)
		assert.Equal(t, "schemaId="+env.schemaID, env.pdp.calls[0].Query)
		assert.Equal(t, "/api/v1/policy/grants", env.pdp.calls[1].Path)
		assert.Equal(t, "consentBypass=false&schemaId="+env.schemaID+"&status=active", env.pdp.calls[1].Query)
	})

	t.Run("Owner member lists policy metadata of their schema", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)
		owner := env.makeMemberOwner(t)

		w := env.serve(callertest.NewAuthenticatedRequest(http.MethodGet, env.url(""), nil, owner))

		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("Member cannot list policy metadata of another member's schema", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)
		owner := env.makeMemberOwner(t)
		otherMemberID := createTestMember(t, env.db, fmt.Sprintf("other-%d@example.com", time.Now().UnixNano()))
		otherSchemaID := createTestSchema(t, env.db, otherMemberID)

		w := env.serve(callertest.NewAuthenticatedRequest(http.MethodGet, fmt.Sprintf("/api/v1/schemas/%s/policy-metadata", otherSchemaID), nil, owner))

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, env.pdp.calls)
	})

	// authn rejects unauthenticated requests before they reach the handler;
	// one that gets here anyway holds no scope and no member
	t.Run("Unauthenticated request is denied", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewUnauthenticatedRequest(http.MethodGet, env.url(""), nil))

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, env.pdp.calls)
	})

	t.Run("Unknown schema returns not found", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodGet, "/api/v1/schemas/sch_missing/policy-metadata", nil))

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, env.pdp.calls)
	})

	t.Run("Method not allowed on collection", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodPost, env.url(""), nil))

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestSchemaPolicyMetadataEndpoints_Patch(t *testing.T) {
	t.Run("Owner member patches a field and only set properties are forwarded", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)
		owner := env.makeMemberOwner(t)

		body := `{"displayName":"Patched","description":null}`
		w := env.serve(callertest.NewAuthenticatedRequest(http.MethodPatch, env.url("/pm-1"), strings.NewReader(body), owner))

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response policy.PolicyMetadataResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "Patched", *response.DisplayName)
		assert.Contains(t, response.AllowList, "client-1")

		calls := env.pdp.writeCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, http.MethodPatch, calls[0].Method)
		assert.Equal(t, "/api/v1/policy/metadata/pm-1", calls[0].Path)
		assert.JSONEq(t, body, calls[0].Body)
	})

	t.Run("Record from another schema is not found and not forwarded", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodPatch, env.url("/pm-other"), strings.NewReader(`{"displayName":"x"}`)))

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, env.pdp.writeCalls())
	})

	t.Run("Empty body is rejected", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodPatch, env.url("/pm-1"), strings.NewReader(`{}`)))

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, env.pdp.calls)
	})

	t.Run("Unknown field is rejected", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodPatch, env.url("/pm-1"), strings.NewReader(`{"fieldName":"x"}`)))

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, env.pdp.calls)
	})

	t.Run("PDP validation error is passed through", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)
		env.pdp.writeStatus = http.StatusBadRequest
		env.pdp.writeBody = `{"error":"unknown policy class \"secret\": must be one of allowlist, allowlist_consent"}`

		w := env.serve(callertest.NewAdminRequest(http.MethodPatch, env.url("/pm-1"), strings.NewReader(`{"policyClass":"secret"}`)))

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), `unknown policy class \"secret\"`)
	})

	t.Run("PDP failure is reported as bad gateway", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)
		env.pdp.writeStatus = http.StatusInternalServerError
		env.pdp.writeBody = `{"error":"internal server error"}`

		w := env.serve(callertest.NewAdminRequest(http.MethodPatch, env.url("/pm-1"), strings.NewReader(`{"displayName":"x"}`)))

		assert.Equal(t, http.StatusBadGateway, w.Code)
	})

	t.Run("Read-only user cannot patch", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewReadOnlyRequest(http.MethodPatch, env.url("/pm-1"), strings.NewReader(`{"displayName":"x"}`)))

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, env.pdp.calls)
	})
}

func TestSchemaPolicyMetadataEndpoints_Delete(t *testing.T) {
	t.Run("Owner member deletes a field's policy metadata", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)
		owner := env.makeMemberOwner(t)

		w := env.serve(callertest.NewAuthenticatedRequest(http.MethodDelete, env.url("/pm-1"), nil, owner))

		require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
		calls := env.pdp.writeCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, http.MethodDelete, calls[0].Method)
		assert.Equal(t, "/api/v1/policy/metadata/pm-1", calls[0].Path)
	})

	t.Run("Member cannot delete from another member's schema", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewMemberRequest(http.MethodDelete, env.url("/pm-1"), nil))

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, env.pdp.calls)
	})

	t.Run("Record from another schema is not found and not forwarded", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodDelete, env.url("/pm-other"), nil))

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, env.pdp.writeCalls())
	})

	t.Run("Method not allowed on record", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodPut, env.url("/pm-1"), bytes.NewBufferString(`{}`)))

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestSchemaPolicyMetadataEndpoints_RevokeAllowListEntry(t *testing.T) {
	t.Run("Owner member revokes an application's access to a field", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)
		owner := env.makeMemberOwner(t)

		w := env.serve(callertest.NewAuthenticatedRequest(http.MethodDelete, env.url("/pm-1/allowlist/client-1"), nil, owner))

		require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
		calls := env.pdp.writeCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, http.MethodPost, calls[0].Method)
		assert.Equal(t, "/api/v1/policy/grants/grant-1/revoke", calls[0].Path)
		assert.Equal(t, policy.GrantStatusRevoked, env.pdp.grants[0].Status)
	})

	t.Run("Application without an active grant returns not found", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		// client-2's grant is already revoked
		w := env.serve(callertest.NewAdminRequest(http.MethodDelete, env.url("/pm-1/allowlist/client-2"), nil))

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "allow-list entry not found")
		assert.Empty(t, env.pdp.writeCalls())
	})

	t.Run("Consent bypass grant is not revoked through the allow-list", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		// client-3 holds only a consent bypass grant
		w := env.serve(callertest.NewAdminRequest(http.MethodDelete, env.url("/pm-1/allowlist/client-3"), nil))

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, env.pdp.writeCalls())
	})

	t.Run("PDP failure while revoking is reported as bad gateway", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)
		env.pdp.writeStatus = http.StatusInternalServerError
		env.pdp.writeBody = `{"error":"internal server error"}`

		w := env.serve(callertest.NewAdminRequest(http.MethodDelete, env.url("/pm-1/allowlist/client-1"), nil))

		assert.Equal(t, http.StatusBadGateway, w.Code)
	})

	t.Run("Record from another schema is not found and not forwarded", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodDelete, env.url("/pm-other/allowlist/client-1"), nil))

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, env.pdp.writeCalls())
	})

	t.Run("Method not allowed on allow-list entry", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodGet, env.url("/pm-1/allowlist/client-1"), nil))

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Unknown sub-path returns not found", func(t *testing.T) {
		env := newPolicyMetadataTestEnv(t)

		w := env.serve(callertest.NewAdminRequest(http.MethodDelete, env.url("/pm-1/other/client-1"), nil))

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSchemaPolicyMetadataEndpoints_PDPUnreachable(t *testing.T) {
	db := setupSQLiteTestDB(t)
	memberID := createTestMember(t, db, fmt.Sprintf("policy-unreachable-%d@example.com", time.Now().UnixNano()))
	schemaID := createTestSchema(t, db, memberID)
	handler := newTestHandler(db)

	w := httptest.NewRecorder()
	newPolicyMetadataMux(handler).ServeHTTP(w, callertest.NewAdminRequest(http.MethodGet, fmt.Sprintf("/api/v1/schemas/%s/policy-metadata", schemaID), nil))

	assert.Equal(t, http.StatusBadGateway, w.Code)
}
