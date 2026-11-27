// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newSchemaIDTestHandler returns a schema Handler on SQLite whose PDP accepts
// every field policy create, and records the schemaIds it was sent.
func newSchemaIDTestHandler(t *testing.T) (*Handler, *gorm.DB, *[]string) {
	var pdpSchemaIDs []string
	pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SchemaID string `json:"schemaId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		pdpSchemaIDs = append(pdpSchemaIDs, body.SchemaID)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"records":[]}`))
	}))
	t.Cleanup(pdp.Close)

	db := setupSQLiteTestDB(t)
	return NewHandler(NewService(db, policy.NewClient(pdp.URL))), db, &pdpSchemaIDs
}

func postSchema(t *testing.T, h *Handler, schemaID, memberID string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(CreateSchemaRequest{
		SchemaID:   schemaID,
		SchemaName: "Identity Registry Person",
		Endpoint:   "http://identity-registry.example/graphql",
		MemberID:   memberID,
		Fields: []policy.FieldPolicyCreateRequestRecord{
			{FieldName: "person.fullName", Source: policy.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	h.CreateSchema(w, callertest.NewAdminRequest(http.MethodPost, "/api/v1/schemas", bytes.NewBuffer(body)))
	return w
}

func TestCreateSchema_SchemaID(t *testing.T) {
	t.Run("uses a caller-supplied schemaId", func(t *testing.T) {
		h, db, pdpSchemaIDs := newSchemaIDTestHandler(t)
		memberID := createTestMember(t, db, fmt.Sprintf("identity-registry-%d@example.com", uniqueID()))

		w := postSchema(t, h, "identity-registry-v1", memberID)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp SchemaResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "identity-registry-v1", resp.SchemaID)
		// The PDP's field policies are keyed by the same ID.
		assert.Equal(t, []string{"identity-registry-v1"}, *pdpSchemaIDs)

		var stored Schema
		assert.NoError(t, db.First(&stored, "schema_id = ?", "identity-registry-v1").Error)
	})

	t.Run("generates a UUID when schemaId is omitted", func(t *testing.T) {
		h, db, _ := newSchemaIDTestHandler(t)
		memberID := createTestMember(t, db, fmt.Sprintf("identity-registry-%d@example.com", uniqueID()))

		w := postSchema(t, h, "", memberID)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp SchemaResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		_, err := uuid.Parse(resp.SchemaID)
		assert.NoError(t, err, "schema ID should be a plain UUID")
	})

	t.Run("rejects a duplicate schemaId with 409", func(t *testing.T) {
		h, db, pdpSchemaIDs := newSchemaIDTestHandler(t)
		memberID := createTestMember(t, db, fmt.Sprintf("identity-registry-%d@example.com", uniqueID()))

		require.Equal(t, http.StatusCreated, postSchema(t, h, "identity-registry-v1", memberID).Code)
		w := postSchema(t, h, "identity-registry-v1", memberID)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), ErrSchemaAlreadyExists.Error())
		// The duplicate never reaches the PDP.
		assert.Len(t, *pdpSchemaIDs, 1)
	})

	t.Run("rejects an invalid schemaId with 400", func(t *testing.T) {
		for _, id := range []string{"ab", "IR-Schema", "registry:schema", "registry/schema", "-registry", "registry schema", strings.Repeat("a", 65)} {
			t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
				h, db, pdpSchemaIDs := newSchemaIDTestHandler(t)
				memberID := createTestMember(t, db, fmt.Sprintf("identity-registry-%d@example.com", uniqueID()))

				w := postSchema(t, h, id, memberID)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), "invalid schemaId")
				assert.Empty(t, *pdpSchemaIDs)
			})
		}
	})

	t.Run("accepts the documented character set", func(t *testing.T) {
		for _, id := range []string{"abc", "identity-registry-v1", "civil.registry_v2", "2024-registry", strings.Repeat("a", 64)} {
			assert.True(t, schemaIDPattern.MatchString(id), id)
		}
	})
}
