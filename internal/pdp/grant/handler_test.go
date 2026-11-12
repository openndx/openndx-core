package grant

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRespondWithServiceError_HidesInternalDetails verifies that unexpected
// service errors are logged without exposing details to clients.
func TestRespondWithServiceError_HidesInternalDetails(t *testing.T) {
	w := httptest.NewRecorder()
	internalErr := errors.New("database connection failed: password=secret")

	respondWithServiceError(w, internalErr)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), internalErr.Error())
}

func TestHandler_NewHandler(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.service)
}

func TestHandler_CreateAccessGrants_InvalidJSON(t *testing.T) {
	// This test doesn't need a database - it only tests JSON parsing
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/grants", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateAccessGrants(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_RevokeAccessGrant(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))
	insertField(t, db, "schema-123", "person.email", true, false)

	grants, err := handler.service.CreateAccessGrants(&AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: GrantDurationTypeOneMonth,
		Records:       []AccessGrantCreateRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
	})
	require.NoError(t, err)
	grantID := grants.Records[0].ID

	revoke := func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/grants/"+id+"/revoke", nil)
		req.SetPathValue("id", id)
		w := httptest.NewRecorder()
		handler.RevokeAccessGrant(w, req)
		return w
	}

	w := revoke(grantID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp AccessGrantResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, grantID, resp.ID)
	assert.Equal(t, GrantStatusRevoked, resp.Status)

	// A revoked grant can't be revoked again.
	assert.Equal(t, http.StatusNotFound, revoke(grantID).Code)
	assert.Equal(t, http.StatusNotFound, revoke("not-a-uuid").Code)
}

func TestHandler_ConsentBypassGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))
	insertField(t, db, "schema-123", "person.nic", false, true)
	insertField(t, db, "schema-123", "person.name", true, false)

	serve := func(method, path, body string, h http.HandlerFunc, pathValues ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		for i := 0; i+1 < len(pathValues); i += 2 {
			req.SetPathValue(pathValues[i], pathValues[i+1])
		}
		w := httptest.NewRecorder()
		h(w, req)
		return w
	}
	grant := func(body string) *httptest.ResponseRecorder {
		return serve(http.MethodPost, "/api/v1/policy/grants", body, handler.CreateAccessGrants)
	}
	bypass := func(field, legalBasis string) *httptest.ResponseRecorder {
		return grant(`{"applicationId":"police","grantDuration":"30d","consentBypass":true,"legalBasis":"` + legalBasis +
			`","records":[{"schemaId":"schema-123","fieldName":"` + field + `"}]}`)
	}

	w := bypass("person.nic", "Police Act s.12")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var created AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	require.Len(t, created.Records, 1)
	id := created.Records[0].ID
	assert.Equal(t, GrantStatusActive, created.Records[0].Status)
	assert.True(t, created.Records[0].ConsentBypass)

	assert.Equal(t, http.StatusBadRequest, bypass("person.nic", " ").Code, "legal basis required")
	assert.Equal(t, http.StatusBadRequest, bypass("person.name", "Police Act s.12").Code, "allowlist field asks for no consent")
	assert.Equal(t, http.StatusNotFound, bypass("person.missing", "Police Act s.12").Code)
	assert.Equal(t, http.StatusBadRequest,
		grant(`{"applicationId":"police","grantDuration":"30d","legalBasis":"Police Act s.12",`+
			`"records":[{"schemaId":"schema-123","fieldName":"person.nic"}]}`).Code,
		"legal basis on a plain grant")

	w = serve(http.MethodGet, "/api/v1/policy/grants/"+id, "", handler.GetAccessGrant, "id", id)
	require.Equal(t, http.StatusOK, w.Code)
	var got AccessGrantResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.True(t, got.ConsentBypass)
	require.NotNil(t, got.LegalBasis)
	assert.Equal(t, "Police Act s.12", *got.LegalBasis)
	assert.Equal(t, http.StatusNotFound,
		serve(http.MethodGet, "/api/v1/policy/grants/x", "", handler.GetAccessGrant, "id", "not-a-uuid").Code)

	w = serve(http.MethodGet, "/api/v1/policy/grants?consentBypass=true", "", handler.ListAccessGrants)
	require.Equal(t, http.StatusOK, w.Code)
	var listed AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&listed))
	require.Len(t, listed.Records, 1)
	assert.Equal(t, id, listed.Records[0].ID)
}

func TestHandler_ListAccessGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))
	insertField(t, db, "schema-123", "person.email", true, false)
	for _, applicationID := range []string{"app-123", "app-456"} {
		_, err := handler.service.CreateAccessGrants(&AccessGrantCreateRequest{
			ApplicationID: applicationID,
			GrantDuration: GrantDurationTypeOneMonth,
			Records:       []AccessGrantCreateRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
		})
		require.NoError(t, err)
	}

	list := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/policy/grants"+query, nil)
		w := httptest.NewRecorder()
		handler.ListAccessGrants(w, req)
		return w
	}

	w := list("?applicationId=app-456&schemaId=schema-123&status=active")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Records, 1)
	assert.Equal(t, "app-456", resp.Records[0].ApplicationID)
	assert.Equal(t, "person.email", resp.Records[0].FieldName)

	assert.Equal(t, http.StatusBadRequest, list("?status=pending").Code)
	assert.Equal(t, http.StatusBadRequest, list("?consentBypass=maybe").Code)
}

func TestHandler_CreateAccessGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(NewService(db))
	insertField(t, db, "schema-123", "person.fullName", true, false)

	tests := []struct {
		name           string
		requestBody    AccessGrantCreateRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Grant access successfully",
			requestBody: AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: GrantDurationTypeOneMonth,
				Records: []AccessGrantCreateRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp AccessGrantListResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
				require.Len(t, resp.Records, 1)
				assert.NotEmpty(t, resp.Records[0].ID)
				assert.Equal(t, "person.fullName", resp.Records[0].FieldName)
				assert.Equal(t, "app-123", resp.Records[0].ApplicationID)
				assert.Equal(t, GrantStatusActive, resp.Records[0].Status)
				assert.NotEmpty(t, resp.Records[0].ExpiresAt)
			},
		},
		{
			name: "Field not found",
			requestBody: AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: GrantDurationTypeOneMonth,
				Records: []AccessGrantCreateRequestRecord{
					{
						FieldName: "person.nonexistent",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusNotFound,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				// The body is the same as before grants had their own package.
				assert.JSONEq(t,
					`{"error":"policy metadata not found for schema_id schema-123 and field_name person.nonexistent"}`,
					w.Body.String())
			},
		},
		{
			name: "Missing application ID",
			requestBody: AccessGrantCreateRequest{
				ApplicationID: "",
				GrantDuration: GrantDurationTypeOneMonth,
				Records:       []AccessGrantCreateRequestRecord{},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "No records",
			requestBody: AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: GrantDurationTypeOneMonth,
				Records:       []AccessGrantCreateRequestRecord{},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Invalid grant duration",
			requestBody: AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: "invalid-duration", // Invalid grant duration
				Records: []AccessGrantCreateRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/grants", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateAccessGrants(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, w)
			}
		})
	}
}
