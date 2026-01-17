package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gov-dx-sandbox/exchange/policy-decision-point/v1/models"
	"github.com/gov-dx-sandbox/exchange/policy-decision-point/v1/testhelpers"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB creates a real PostgreSQL database connection for integration-style tests.
// NOTE: These tests use real database connections because they test handler behavior
// with actual database operations. All database testing is done via integration tests
// in tests/integration/.
//
// These tests will be skipped if a database connection is not available.
func setupTestDB(t *testing.T) *gorm.DB {
	db := testhelpers.SetupPostgresTestDB(t)
	if db == nil {
		t.SkipNow()
	}
	return db
}

func TestHandler_CreatePolicyMetadata_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/metadata", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreatePolicyMetadata(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_UpdateAllowList_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/update-allowlist", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.UpdateAllowList(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_GetPolicyDecision_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/decide", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.GetPolicyDecision(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_SetupRoutes(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	mux := http.NewServeMux()
	handler.SetupRoutes(mux)

	// Verify routes are registered
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/metadata", bytes.NewBuffer([]byte("{}")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should not return 404 (route exists)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestHandler_NewHandler(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.policyService)
}

func TestHandler_CreatePolicyMetadata(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    models.PolicyMetadataCreateRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Create new policy metadata successfully",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       testhelpers.StringPtr("Full Name"),
						Description:       testhelpers.StringPtr("Complete name"),
						Source:            models.SourcePrimary,
						IsOwner:           true,
						AccessControlType: models.AccessControlTypePublic,
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.Records) != 1 {
					t.Errorf("Expected 1 record, got %d", len(resp.Records))
				}
				if resp.Records[0].FieldName != "person.fullName" {
					t.Errorf("Expected fieldName person.fullName, got %s", resp.Records[0].FieldName)
				}
			},
		},
		{
			name: "Update existing policy metadata",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       testhelpers.StringPtr("Full Name Updated"),
						Description:       testhelpers.StringPtr("Updated description"),
						Source:            models.SourcePrimary,
						IsOwner:           true,
						AccessControlType: models.AccessControlTypeRestricted,
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.Records) != 1 {
					t.Errorf("Expected 1 record, got %d", len(resp.Records))
				}
				if resp.Records[0].AccessControlType != models.AccessControlTypeRestricted {
					t.Errorf("Expected AccessControlType restricted, got %s", resp.Records[0].AccessControlType)
				}
			},
		},
		{
			name: "Empty request body",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "",
				Records:  []models.PolicyMetadataCreateRequestRecord{},
			},
			expectedStatus: http.StatusCreated, // Handler doesn't validate, service will handle
		},
		{
			name: "Service error - invalid field configuration",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.invalid",
						Source:            models.SourcePrimary,
						IsOwner:           false, // Invalid: isOwner false but no owner specified
						AccessControlType: models.AccessControlTypePublic,
					},
				},
			},
			expectedStatus: http.StatusInternalServerError,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				// Should return error response
				assert.Contains(t, w.Body.String(), "owner")
			},
		},
		{
			name: "Create with multiple records",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						DisplayName:       testhelpers.StringPtr("Full Name"),
						Source:            models.SourcePrimary,
						IsOwner:           true,
						AccessControlType: models.AccessControlTypePublic,
					},
					{
						FieldName:         "person.email",
						DisplayName:       testhelpers.StringPtr("Email"),
						Source:            models.SourcePrimary,
						IsOwner:           true,
						AccessControlType: models.AccessControlTypeRestricted,
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.Records) != 2 {
					t.Errorf("Expected 2 records, got %d", len(resp.Records))
				}
			},
		},
		{
			name: "Create with fallback source",
			requestBody: models.PolicyMetadataCreateRequest{
				SchemaID: "schema-123",
				Records: []models.PolicyMetadataCreateRequestRecord{
					{
						FieldName:         "person.fullName",
						Source:            models.SourceFallback,
						IsOwner:           true,
						AccessControlType: models.AccessControlTypePublic,
					},
				},
			},
			expectedStatus: http.StatusCreated,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.PolicyMetadataCreateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if resp.Records[0].Source != models.SourceFallback {
					t.Errorf("Expected source fallback, got %s", resp.Records[0].Source)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset database for each test
			db := setupTestDB(t)
			handler := NewHandler(db)

			// Create initial record for update test
			if tt.name == "Update existing policy metadata" {
				initialReq := models.PolicyMetadataCreateRequest{
					SchemaID: "schema-123",
					Records: []models.PolicyMetadataCreateRequestRecord{
						{
							FieldName:         "person.fullName",
							DisplayName:       testhelpers.StringPtr("Full Name"),
							Source:            models.SourcePrimary,
							IsOwner:           true,
							AccessControlType: models.AccessControlTypePublic,
						},
					},
				}
				handler.policyService.CreatePolicyMetadata(&initialReq)
			}

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/metadata", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreatePolicyMetadata(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, w)
			}
		})
	}
}

func TestHandler_UpdateAllowList(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	// Create initial policy metadata
	createReq := models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.fullName",
				DisplayName:       testhelpers.StringPtr("Full Name"),
				Source:            models.SourcePrimary,
				IsOwner:           true,
				AccessControlType: models.AccessControlTypePublic,
			},
		},
	}
	handler.policyService.CreatePolicyMetadata(&createReq)

	tests := []struct {
		name           string
		requestBody    models.AllowListUpdateRequest
		expectedStatus int
		validateFunc   func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		{
			name: "Update allow list successfully",
			requestBody: models.AllowListUpdateRequest{
				ApplicationID: "app-123",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records: []models.AllowListUpdateRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusOK,
			validateFunc: func(t *testing.T, w *httptest.ResponseRecorder) {
				var resp models.AllowListUpdateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if len(resp.Records) != 1 {
					t.Errorf("Expected 1 record, got %d", len(resp.Records))
				}
				if resp.Records[0].FieldName != "person.fullName" {
					t.Errorf("Expected fieldName person.fullName, got %s", resp.Records[0].FieldName)
				}
				if resp.Records[0].ExpiresAt == "" {
					t.Error("Expected expiresAt to be set")
				}
			},
		},
		{
			name: "Field not found",
			requestBody: models.AllowListUpdateRequest{
				ApplicationID: "app-123",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records: []models.AllowListUpdateRequestRecord{
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
			requestBody: models.AllowListUpdateRequest{
				ApplicationID: "",
				GrantDuration: models.GrantDurationTypeOneMonth,
				Records:       []models.AllowListUpdateRequestRecord{},
			},
			expectedStatus: http.StatusOK, // Handler doesn't validate, service will handle
		},
		{
			name: "Service error - invalid grant duration",
			requestBody: models.AllowListUpdateRequest{
				ApplicationID: "app-123",
				GrantDuration: "invalid-duration", // Invalid grant duration
				Records: []models.AllowListUpdateRequestRecord{
					{
						FieldName: "person.fullName",
						SchemaID:  "schema-123",
					},
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/update-allowlist", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.UpdateAllowList(w, req)

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
	handler := NewHandler(db)

	// Create policy metadata with allow list
	createReq := models.PolicyMetadataCreateRequest{
		SchemaID: "schema-123",
		Records: []models.PolicyMetadataCreateRequestRecord{
			{
				FieldName:         "person.fullName",
				DisplayName:       testhelpers.StringPtr("Full Name"),
				Source:            models.SourcePrimary,
				IsOwner:           true,
				AccessControlType: models.AccessControlTypePublic,
			},
			{
				FieldName:         "person.nic",
				DisplayName:       testhelpers.StringPtr("NIC"),
				Source:            models.SourcePrimary,
				IsOwner:           false,
				AccessControlType: models.AccessControlTypeRestricted,
				Owner:             testhelpers.OwnerPtr(models.OwnerCitizen),
			},
		},
	}
	_, err := handler.policyService.CreatePolicyMetadata(&createReq)
	if err != nil {
		t.Fatalf("Failed to create policy metadata: %v", err)
	}

	// Update allow list for authorized field
	updateReq := models.AllowListUpdateRequest{
		ApplicationID: "app-123",
		GrantDuration: models.GrantDurationTypeOneMonth,
		Records: []models.AllowListUpdateRequestRecord{
			{
				FieldName: "person.fullName",
				SchemaID:  "schema-123",
			},
		},
	}
	_, err = handler.policyService.UpdateAllowList(&updateReq)
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
				updateReq := models.AllowListUpdateRequest{
					ApplicationID: "app-123",
					GrantDuration: models.GrantDurationTypeOneMonth,
					Records: []models.AllowListUpdateRequestRecord{
						{
							FieldName: "person.nic",
							SchemaID:  "schema-123",
						},
					},
				}
				_, err := handler.policyService.UpdateAllowList(&updateReq)
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

func TestHandler_handlePolicyService(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(db)

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "POST /api/v1/policy/metadata",
			method:         http.MethodPost,
			path:           "/api/v1/policy/metadata",
			expectedStatus: http.StatusCreated, // Endpoint exists, will process request
		},
		{
			name:           "POST /api/v1/policy/update-allowlist",
			method:         http.MethodPost,
			path:           "/api/v1/policy/update-allowlist",
			expectedStatus: http.StatusOK, // Endpoint exists, will process request
		},
		{
			name:           "POST /api/v1/policy/decide",
			method:         http.MethodPost,
			path:           "/api/v1/policy/decide",
			expectedStatus: http.StatusOK, // Endpoint exists, will process request
		},
		{
			name:           "GET /api/v1/policy/metadata - Method not allowed",
			method:         http.MethodGet,
			path:           "/api/v1/policy/metadata",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "PUT /api/v1/policy/metadata - Method not allowed",
			method:         http.MethodPut,
			path:           "/api/v1/policy/metadata",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "DELETE /api/v1/policy/metadata - Method not allowed",
			method:         http.MethodDelete,
			path:           "/api/v1/policy/metadata",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "GET /api/v1/policy/update-allowlist - Method not allowed",
			method:         http.MethodGet,
			path:           "/api/v1/policy/update-allowlist",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "GET /api/v1/policy/decide - Method not allowed",
			method:         http.MethodGet,
			path:           "/api/v1/policy/decide",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid path - single segment",
			method:         http.MethodPost,
			path:           "/api/v1/policy/invalid",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Invalid path - multiple segments",
			method:         http.MethodPost,
			path:           "/api/v1/policy/metadata/extra",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Invalid path - empty after prefix",
			method:         http.MethodPost,
			path:           "/api/v1/policy/",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Invalid path - three segments",
			method:         http.MethodPost,
			path:           "/api/v1/policy/metadata/extra/segment",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "PATCH method not allowed",
			method:         http.MethodPatch,
			path:           "/api/v1/policy/metadata",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "OPTIONS method not allowed",
			method:         http.MethodOptions,
			path:           "/api/v1/policy/metadata",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBuffer([]byte("{}")))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.handlePolicyService(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}
