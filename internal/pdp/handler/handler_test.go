package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/pdp/fieldpolicy"
	"github.com/openndx/openndx-core/internal/pdp/grant"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/pdptest"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for unit testing.
func setupTestDB(t *testing.T) *gorm.DB {
	return pdptest.SetupTestDB(t)
}

func TestHandler_GetPolicyDecision_InvalidJSON(t *testing.T) {
	// This test doesn't need a database - it only tests JSON parsing
	db := setupTestDB(t)
	handler := NewHandler(fieldpolicy.NewService(db), grant.NewService(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/decide", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.GetPolicyDecision(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_NewHandler(t *testing.T) {
	db := setupTestDB(t)
	handler := NewHandler(fieldpolicy.NewService(db), grant.NewService(db))
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.policyService)
}

func TestHandler_GetPolicyDecision(t *testing.T) {
	db := setupTestDB(t)
	grants := grant.NewService(db)
	handler := NewHandler(fieldpolicy.NewService(db), grants)

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
	updateReq := grant.AccessGrantCreateRequest{
		ApplicationID: "app-123",
		GrantDuration: grant.GrantDurationTypeOneMonth,
		Records: []grant.AccessGrantCreateRequestRecord{
			{
				FieldName: "person.fullName",
				SchemaID:  "schema-123",
			},
		},
	}
	_, err = grants.CreateAccessGrants(&updateReq)
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
				updateReq := grant.AccessGrantCreateRequest{
					ApplicationID: "app-123",
					GrantDuration: grant.GrantDurationTypeOneMonth,
					Records: []grant.AccessGrantCreateRequestRecord{
						{
							FieldName: "person.nic",
							SchemaID:  "schema-123",
						},
					},
				}
				_, err := grants.CreateAccessGrants(&updateReq)
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
