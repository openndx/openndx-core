package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/pdp/fieldpolicy"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/pdptest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for unit testing.
func setupTestDB(t *testing.T) *gorm.DB {
	return pdptest.SetupTestDB(t)
}

// TestRespondWithPolicyServiceError_HidesInternalDetails verifies that
// unexpected service errors are logged without exposing details to clients.
func TestRespondWithPolicyServiceError_HidesInternalDetails(t *testing.T) {
	w := httptest.NewRecorder()
	internalErr := errors.New("database connection failed: password=secret")

	respondWithPolicyServiceError(w, internalErr)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), internalErr.Error())
}

func TestHandler_CreateAccessGrants_InvalidJSON(t *testing.T) {
	// This test doesn't need a database - it only tests JSON parsing
	db := setupTestDB(t)
	handler := NewHandler(db, fieldpolicy.NewService(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/grants", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateAccessGrants(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetPolicyDecision_InvalidJSON(t *testing.T) {
	// This test doesn't need a database - it only tests JSON parsing
	db := setupTestDB(t)
	handler := NewHandler(db, fieldpolicy.NewService(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/decide", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.GetPolicyDecision(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_NewHandler(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db, fieldpolicy.NewService(db))
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.policyService)
}

func TestHandler_RevokeAccessGrant(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db, fieldpolicy.NewService(db))

	_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

	grants, err := handler.policyService.CreateAccessGrants(&models.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records:       []models.AccessGrantCreateRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
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
	var resp models.AccessGrantResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, grantID, resp.ID)
	assert.Equal(t, models.GrantStatusRevoked, resp.Status)

	// A revoked grant can't be revoked again.
	assert.Equal(t, http.StatusNotFound, revoke(grantID).Code)
	assert.Equal(t, http.StatusNotFound, revoke("not-a-uuid").Code)
}

func TestHandler_ConsentBypassGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db, fieldpolicy.NewService(db))

	_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.nic", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(false), RequiresConsent: new(true)},
			{FieldName: "person.name", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)

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
	var created models.AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	require.Len(t, created.Records, 1)
	id := created.Records[0].ID
	assert.Equal(t, models.GrantStatusActive, created.Records[0].Status)
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
	var got models.AccessGrantResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.True(t, got.ConsentBypass)
	require.NotNil(t, got.LegalBasis)
	assert.Equal(t, "Police Act s.12", *got.LegalBasis)
	assert.Equal(t, http.StatusNotFound,
		serve(http.MethodGet, "/api/v1/policy/grants/x", "", handler.GetAccessGrant, "id", "not-a-uuid").Code)

	w = serve(http.MethodGet, "/api/v1/policy/grants?consentBypass=true", "", handler.ListAccessGrants)
	require.Equal(t, http.StatusOK, w.Code)
	var listed models.AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&listed))
	require.Len(t, listed.Records, 1)
	assert.Equal(t, id, listed.Records[0].ID)
}

func TestHandler_ListAccessGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db, fieldpolicy.NewService(db))

	_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&fieldpolicy.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
			{FieldName: "person.email", Source: fieldpolicy.SourcePrimary, RequiresAllowlist: new(true), RequiresConsent: new(false)},
		},
	})
	require.NoError(t, err)
	for _, applicationID := range []string{"app-123", "app-456"} {
		_, err = handler.policyService.CreateAccessGrants(&models.AccessGrantCreateRequest{
			ApplicationID: applicationID,
			GrantDuration: models.GrantDurationTypeOneMonth,
			Records:       []models.AccessGrantCreateRequestRecord{{FieldName: "person.email", SchemaID: "schema-123"}},
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
	var resp models.AccessGrantListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Records, 1)
	assert.Equal(t, "app-456", resp.Records[0].ApplicationID)
	assert.Equal(t, "person.email", resp.Records[0].FieldName)

	assert.Equal(t, http.StatusBadRequest, list("?status=pending").Code)
	assert.Equal(t, http.StatusBadRequest, list("?consentBypass=maybe").Code)
}

func TestHandler_CreateAccessGrants(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db, fieldpolicy.NewService(db))

	// Create initial policy metadata
	createReq := fieldpolicy.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.fullName",
				DisplayName:       pdptest.StringPtr("Full Name"),
				Source:            fieldpolicy.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
		},
	}
	_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&createReq)
	require.NoError(t, err)

	tests := []struct {
		name           string
		requestBody    models.AccessGrantCreateRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Grant access successfully",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records: []models.AccessGrantCreateRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.AccessGrantListResponse
				require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
				require.Len(t, resp.Records, 1)
				assert.NotEmpty(t, resp.Records[0].ID)
				assert.Equal(t, "person.fullName", resp.Records[0].FieldName)
				assert.Equal(t, "app-123", resp.Records[0].ApplicationID)
				assert.Equal(t, models.GrantStatusActive, resp.Records[0].Status)
				assert.NotEmpty(t, resp.Records[0].ExpiresAt)
			},
		},
		{
			name: "Field not found",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records: []models.AccessGrantCreateRequestRecord{
					{
						FieldName: "person.nonexistent",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name: "Missing application ID",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records:       []models.AccessGrantCreateRequestRecord{},
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "No records",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records:       []models.AccessGrantCreateRequestRecord{},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Invalid grant duration",
			requestBody: models.AccessGrantCreateRequest{
				ApplicationID: "app-123",
				GrantDuration: "invalid-duration", // Invalid grant duration
				Records: []models.AccessGrantCreateRequestRecord{
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

func TestHandler_GetPolicyDecision(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db, fieldpolicy.NewService(db))

	// Create policy metadata with allow list
	createReq := fieldpolicy.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []fieldpolicy.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.fullName",
				DisplayName:       pdptest.StringPtr("Full Name"),
				Source:            fieldpolicy.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(false),
			},
			{
				FieldName:         "person.nic",
				DisplayName:       pdptest.StringPtr("NIC"),
				Source:            fieldpolicy.SourcePrimary,
				RequiresAllowlist: new(true), RequiresConsent: new(true),
				Owner: new(fieldpolicy.OwnerCitizen),
			},
		},
	}
	_, err := fieldpolicy.NewService(db).CreatePolicyMetadata(&createReq)
	if err != nil {
		t.Fatalf("Failed to create policy metadata: %v", err)
	}

	// Update allow list for authorized field
	updateReq := models.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records: []models.AccessGrantCreateRequestRecord{
			{
				FieldName: "person.fullName",
				SchemaID:  "schema-123",
			},
		},
	}
	_, err = handler.policyService.CreateAccessGrants(&updateReq)
	if err != nil {
		t.Fatalf("Failed to update allow list: %v", err)
	}

	tests := []struct {
		name           string
		requestBody    models.PolicyDecisionRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Authorized request",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyDecisionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if !resp.AppAuthorized {
					t.Error("Expected appAuthorized to be true")
				}
				if len(resp.UnauthorizedFields) > 0 {
					t.Errorf("Expected no unauthorized fields, got %d", len(resp.UnauthorizedFields))
				}
			},
		},
		{
			name: "Unauthorized request - not in allow list",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-456", // Different app, not in allow list
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyDecisionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.AppAuthorized {
					t.Error("Expected appAuthorized to be false")
				}
				if len(resp.UnauthorizedFields) != 1 {
					t.Errorf("Expected 1 unauthorized field, got %d", len(resp.UnauthorizedFields))
				}
			},
		},
		{
			name: "Unauthorized request - restricted field not in allow list",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.nic",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyDecisionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.AppAuthorized {
					t.Error("Expected appAuthorized to be false")
				}
				if len(resp.UnauthorizedFields) != 1 {
					t.Errorf("Expected 1 unauthorized field, got %d", len(resp.UnauthorizedFields))
				}
			},
		},
		{
			name: "Consent required - restricted field in allow list",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.nic",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyDecisionResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if !resp.AppAuthorized {
					t.Error("Expected appAuthorized to be true (field is in allow list)")
				}
				if !resp.AppRequiresOwnerConsent {
					t.Error("Expected appRequiresOwnerConsent to be true")
				}
				if len(resp.ConsentRequiredFields) != 1 {
					t.Errorf("Expected 1 consent required field, got %d", len(resp.ConsentRequiredFields))
				}
				if len(resp.UnauthorizedFields) > 0 {
					t.Errorf("Expected no unauthorized fields, got %d", len(resp.UnauthorizedFields))
				}
			},
		},
		{
			name: "Field not found",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.nonexistent",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "Empty request body",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID:  "",
				RequiredFields: []models.PolicyDecisionRequestRecord{},
			},
			expectedStatus: http.StatusBadRequest, // Handler validates required fields
		},
		{
			name: "Service error - schema not found",
			requestBody: models.PolicyDecisionRequest{
				ApplicationID: "app-123",
				RequiredFields: []models.PolicyDecisionRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "nonexistent-schema",
					},
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Update allow list for consent-required test case before making the request
			if tt.name == "Consent required - restricted field in allow list" {
				updateReq := models.AccessGrantCreateRequest{
					ApplicationID: "app-123",
					GrantDuration: models.GrantDurationTypeOneMonth,
					Records: []models.AccessGrantCreateRequestRecord{
						{
							FieldName: "person.nic",
							SchemaID:  "schema-123",
						},
					},
				}
				_, err := handler.policyService.CreateAccessGrants(&updateReq)
				if err != nil {
					t.Fatalf("Failed to update allow list for test setup: %v", err)
				}
			}

			// Use the same handler instance for all operations
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/decide", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.GetPolicyDecision(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
				return // Skip validation if status doesn't match
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, w)
			}
		})
	}
}
