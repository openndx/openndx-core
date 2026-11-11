package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newSchemaIDTestHandler returns a schema Handler on SQLite whose PDP accepts
// every policy-metadata create, and records the schemaIds it was sent.
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
		SchemaName: "DRP Person Registry",
		Endpoint:   "http://drp.example/graphql",
		MemberID:   memberID,
		Fields: []policy.PolicyMetadataCreateRequestRecord{
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
		memberID := createTestMember(t, db, fmt.Sprintf("drp-%d@example.com", time.Now().UnixNano()))

		w := postSchema(t, h, "drp-person-v1", memberID)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp SchemaResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "drp-person-v1", resp.SchemaID)
		// The PDP's policy metadata is keyed by the same ID.
		assert.Equal(t, []string{"drp-person-v1"}, *pdpSchemaIDs)

		var stored Schema
		assert.NoError(t, db.First(&stored, "schema_id = ?", "drp-person-v1").Error)
	})

	t.Run("generates a UUID when schemaId is omitted", func(t *testing.T) {
		h, db, _ := newSchemaIDTestHandler(t)
		memberID := createTestMember(t, db, fmt.Sprintf("drp-%d@example.com", time.Now().UnixNano()))

		w := postSchema(t, h, "", memberID)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var resp SchemaResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		_, err := uuid.Parse(resp.SchemaID)
		assert.NoError(t, err, "schema ID should be a plain UUID")
	})

	t.Run("rejects a duplicate schemaId with 409", func(t *testing.T) {
		h, db, pdpSchemaIDs := newSchemaIDTestHandler(t)
		memberID := createTestMember(t, db, fmt.Sprintf("drp-%d@example.com", time.Now().UnixNano()))

		require.Equal(t, http.StatusCreated, postSchema(t, h, "drp-person-v1", memberID).Code)
		w := postSchema(t, h, "drp-person-v1", memberID)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), ErrSchemaAlreadyExists.Error())
		// The duplicate never reaches the PDP.
		assert.Len(t, *pdpSchemaIDs, 1)
	})

	t.Run("rejects an invalid schemaId with 400", func(t *testing.T) {
		for _, id := range []string{"ab", "DRP-Schema", "drp:schema", "drp/schema", "-drp", "drp schema", strings.Repeat("a", 65)} {
			t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
				h, db, pdpSchemaIDs := newSchemaIDTestHandler(t)
				memberID := createTestMember(t, db, fmt.Sprintf("drp-%d@example.com", time.Now().UnixNano()))

				w := postSchema(t, h, id, memberID)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), "invalid schemaId")
				assert.Empty(t, *pdpSchemaIDs)
			})
		}
	})

	t.Run("accepts the documented character set", func(t *testing.T) {
		for _, id := range []string{"abc", "drp-person-v1", "rgd.person_v2", "2024-registry", strings.Repeat("a", 64)} {
			assert.True(t, schemaIDPattern.MatchString(id), id)
		}
	})
}
