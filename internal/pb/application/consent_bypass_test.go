package application

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNSW/core/authz"
	"github.com/openndx/openndx-core/internal/pb/caller"
	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
	"github.com/openndx/openndx-core/internal/pb/idp/idptest"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bypassPDP is an in-process PDP serving access grants from memory.
type bypassPDP struct {
	grants []policy.AccessGrant
	// failWith, when set, is the status every write request answers with
	failWith int
	requests []string
	// created is the last grant creation request
	created policy.AccessGrantCreateRequest
}

func (f *bypassPDP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	write := func(status int, v any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	find := func(id string) *policy.AccessGrant {
		for i := range f.grants {
			if f.grants[i].ID == id {
				return &f.grants[i]
			}
		}
		return nil
	}

	if f.failWith != 0 && r.Method == http.MethodPost {
		write(f.failWith, map[string]string{"error": "pdp says no"})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/policy/grants")
	switch {
	case r.Method == http.MethodPost && path == "":
		var req policy.AccessGrantCreateRequest
		_ = json.Unmarshal(body, &req)
		f.created = req
		created := []policy.AccessGrant{}
		for _, record := range req.Records {
			legalBasis := req.LegalBasis
			grant := policy.AccessGrant{
				ID: fmt.Sprintf("grant-%d", len(f.grants)+1), SchemaID: record.SchemaID, FieldName: record.FieldName,
				ApplicationID: req.ApplicationID, Status: policy.GrantStatusActive, ConsentBypass: req.ConsentBypass,
				LegalBasis: &legalBasis, ExpiresAt: "2026-12-10T00:00:00Z",
			}
			f.grants = append(f.grants, grant)
			created = append(created, grant)
		}
		write(http.StatusOK, policy.AccessGrantListResponse{Records: created})

	case r.Method == http.MethodGet && path == "":
		query := r.URL.Query()
		listed := []policy.AccessGrant{}
		for _, grant := range f.grants {
			if (query.Get("applicationId") == "" || query.Get("applicationId") == grant.ApplicationID) &&
				(query.Get("status") == "" || query.Get("status") == grant.Status) &&
				(query.Get("consentBypass") == "" || query.Get("consentBypass") == fmt.Sprint(grant.ConsentBypass)) {
				listed = append(listed, grant)
			}
		}
		write(http.StatusOK, policy.AccessGrantListResponse{Records: listed})

	case r.Method == http.MethodGet:
		if grant := find(strings.TrimPrefix(path, "/")); grant != nil {
			write(http.StatusOK, grant)
			return
		}
		write(http.StatusNotFound, map[string]string{"error": "access grant not found"})

	case r.Method == http.MethodPost && strings.HasSuffix(path, "/revoke"):
		grant := find(strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/revoke"))
		if grant == nil {
			write(http.StatusNotFound, map[string]string{"error": "access grant not found"})
			return
		}
		grant.Status = policy.GrantStatusRevoked
		write(http.StatusOK, grant)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type bypassTestEnv struct {
	pdp           *bypassPDP
	mux           *http.ServeMux
	applicationID string
}

// newBypassTestEnv registers the consent bypass routes with the scopes
// cmd/pb/main.go gives them, over a fake PDP and one application whose IdP
// client is "police-client".
func newBypassTestEnv(t *testing.T) *bypassTestEnv {
	db := setupSQLiteTestDB(t)
	memberID := createTestMember(t, db, "bypass@example.com")
	applicationID := createTestApplicationWithClientID(t, db, memberID, "police-client")

	pdp := &bypassPDP{}
	server := httptest.NewServer(pdp)
	t.Cleanup(server.Close)
	handler := NewHandler(NewService(db, policy.NewClient(server.URL), &idptest.Mock{}))

	authzr, err := authz.New(caller.PrincipalFromContext)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/applications/{applicationId}/consent-bypass-grants", authzr.RequireScope(caller.ScopeConsentBypassWrite)(http.HandlerFunc(handler.GrantConsentBypass)))
	mux.Handle("GET /api/v1/consent-bypass-grants", authzr.RequireScope(caller.ScopeConsentBypassRead)(http.HandlerFunc(handler.ListConsentBypassGrants)))
	mux.Handle("POST /api/v1/consent-bypass-grants/{grantId}/revoke", authzr.RequireScope(caller.ScopeConsentBypassWrite)(http.HandlerFunc(handler.RevokeConsentBypass)))

	return &bypassTestEnv{pdp: pdp, mux: mux, applicationID: applicationID}
}

func (e *bypassTestEnv) serve(method, path, body string, user callertest.TestUser) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, callertest.NewAuthenticatedRequest(method, path, strings.NewReader(body), user))
	return w
}

func (e *bypassTestEnv) grant(user callertest.TestUser) *httptest.ResponseRecorder {
	return e.serve(http.MethodPost, "/api/v1/applications/"+e.applicationID+"/consent-bypass-grants",
		`{"fields":[{"schemaId":"drp","fieldName":"person.nic"}],"legalBasis":"Police Act s.12","grantDuration":"30d"}`, user)
}

func TestConsentBypassEndpoints_GrantListRevoke(t *testing.T) {
	env := newBypassTestEnv(t)

	w := env.grant(callertest.AdminUser)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var created struct {
		Items []ConsentBypassGrantResponse `json:"items"`
		Count int                          `json:"count"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Equal(t, 1, created.Count)
	grant := created.Items[0]
	assert.Equal(t, policy.GrantStatusActive, grant.Status)
	assert.Equal(t, env.applicationID, grant.ApplicationID)
	assert.Equal(t, "police-client", grant.IdpClientID, "PDP grants are keyed by the IdP client")
	assert.Equal(t, "Police Act s.12", *grant.LegalBasis)

	// One PDP call creates the grant as a consent bypass grant.
	assert.Equal(t, []string{"POST /api/v1/policy/grants"}, env.pdp.requests)
	assert.True(t, env.pdp.created.ConsentBypass)
	assert.Equal(t, "police-client", env.pdp.created.ApplicationID)

	w = env.serve(http.MethodGet, "/api/v1/consent-bypass-grants?status=active", "", callertest.AdminUser)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), grant.ID)

	w = env.serve(http.MethodPost, "/api/v1/consent-bypass-grants/"+grant.ID+"/revoke", "", callertest.AdminUser)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var revoked ConsentBypassGrantResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &revoked))
	assert.Equal(t, policy.GrantStatusRevoked, revoked.Status)
	assert.Equal(t, env.applicationID, revoked.ApplicationID)
}

func TestConsentBypassEndpoints_Scopes(t *testing.T) {
	env := newBypassTestEnv(t)

	// Members and cross-member readers hold no consent bypass scope.
	for _, user := range []callertest.TestUser{callertest.MemberUser, callertest.ReadOnlyUser} {
		assert.Equal(t, http.StatusForbidden, env.grant(user).Code)
		assert.Equal(t, http.StatusForbidden, env.serve(http.MethodGet, "/api/v1/consent-bypass-grants", "", user).Code)
	}

	// Reading doesn't allow granting.
	reader := callertest.CreateCustomTestUser("reader", "reader@test.com", "", []string{caller.ScopeConsentBypassRead})
	assert.Equal(t, http.StatusForbidden, env.grant(reader).Code)
	assert.Equal(t, http.StatusOK, env.serve(http.MethodGet, "/api/v1/consent-bypass-grants", "", reader).Code)
	assert.Empty(t, env.pdp.grants)

	// The write scope alone is enough to grant and revoke.
	writer := callertest.CreateCustomTestUser("writer", "writer@test.com", "", []string{caller.ScopeConsentBypassWrite})
	require.Equal(t, http.StatusOK, env.grant(writer).Code)
	grantID := env.pdp.grants[0].ID
	for _, user := range []callertest.TestUser{callertest.MemberUser, callertest.ReadOnlyUser, reader} {
		assert.Equal(t, http.StatusForbidden,
			env.serve(http.MethodPost, "/api/v1/consent-bypass-grants/"+grantID+"/revoke", "", user).Code)
	}
	assert.Equal(t, policy.GrantStatusActive, env.pdp.grants[0].Status)
	assert.Equal(t, http.StatusOK,
		env.serve(http.MethodPost, "/api/v1/consent-bypass-grants/"+grantID+"/revoke", "", writer).Code)
}

func TestConsentBypassEndpoints_Errors(t *testing.T) {
	env := newBypassTestEnv(t)

	tests := map[string]struct {
		path, body string
		want       int
	}{
		"unknown application": {"/api/v1/applications/app-missing/consent-bypass-grants",
			`{"fields":[{"schemaId":"drp","fieldName":"person.nic"}],"legalBasis":"x","grantDuration":"30d"}`, http.StatusNotFound},
		"invalid body": {"/api/v1/applications/" + env.applicationID + "/consent-bypass-grants", `{`, http.StatusBadRequest},
		"no fields": {"/api/v1/applications/" + env.applicationID + "/consent-bypass-grants",
			`{"fields":[],"legalBasis":"x","grantDuration":"30d"}`, http.StatusBadRequest},
		"no legal basis": {"/api/v1/applications/" + env.applicationID + "/consent-bypass-grants",
			`{"fields":[{"schemaId":"drp","fieldName":"person.nic"}],"legalBasis":" ","grantDuration":"30d"}`, http.StatusBadRequest},
		"no grant duration": {"/api/v1/applications/" + env.applicationID + "/consent-bypass-grants",
			`{"fields":[{"schemaId":"drp","fieldName":"person.nic"}],"legalBasis":"x"}`, http.StatusBadRequest},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w := env.serve(http.MethodPost, tt.path, tt.body, callertest.AdminUser)
			assert.Equal(t, tt.want, w.Code, w.Body.String())
		})
	}
	assert.Empty(t, env.pdp.requests, "invalid requests never reach the PDP")

	// Revoking an unknown grant, or a plain grant, is not found.
	env.pdp.grants = append(env.pdp.grants, policy.AccessGrant{ID: "plain-1", Status: policy.GrantStatusActive})
	for _, id := range []string{"missing", "plain-1"} {
		assert.Equal(t, http.StatusNotFound,
			env.serve(http.MethodPost, "/api/v1/consent-bypass-grants/"+id+"/revoke", "", callertest.AdminUser).Code)
	}
	assert.Equal(t, policy.GrantStatusActive, env.pdp.grants[0].Status, "a plain grant is never revoked here")

	// A PDP client error is passed through; a PDP failure is a bad gateway.
	env.pdp.failWith = http.StatusBadRequest
	assert.Equal(t, http.StatusBadRequest, env.grant(callertest.AdminUser).Code)
	env.pdp.failWith = http.StatusInternalServerError
	assert.Equal(t, http.StatusBadGateway, env.grant(callertest.AdminUser).Code)
}

func TestConsentBypassEndpoints_ListByApplication(t *testing.T) {
	env := newBypassTestEnv(t)
	require.Equal(t, http.StatusOK, env.grant(callertest.AdminUser).Code)
	env.pdp.grants = append(env.pdp.grants,
		policy.AccessGrant{ID: "other", ApplicationID: "other-client", ConsentBypass: true, Status: policy.GrantStatusActive},
		policy.AccessGrant{ID: "plain", ApplicationID: "police-client", Status: policy.GrantStatusActive},
	)

	w := env.serve(http.MethodGet, "/api/v1/consent-bypass-grants?applicationId="+env.applicationID, "", callertest.AdminUser)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listed struct {
		Items []ConsentBypassGrantResponse `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	require.Len(t, listed.Items, 1, "plain grants are not listed")
	assert.Equal(t, env.applicationID, listed.Items[0].ApplicationID)

	// Without a filter, a grant whose client is no PB application has no applicationId.
	w = env.serve(http.MethodGet, "/api/v1/consent-bypass-grants", "", callertest.AdminUser)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	require.Len(t, listed.Items, 2)
	assert.Equal(t, "", listed.Items[1].ApplicationID)
	assert.Equal(t, "other-client", listed.Items[1].IdpClientID)

	assert.Equal(t, http.StatusNotFound,
		env.serve(http.MethodGet, "/api/v1/consent-bypass-grants?applicationId=app-missing", "", callertest.AdminUser).Code)
}
