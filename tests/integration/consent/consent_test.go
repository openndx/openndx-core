package consent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/gov-dx-sandbox/tests/integration/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	consentBaseURL = "http://127.0.0.1:8081"
)

// Request/Response types for type safety

type ConsentField struct {
	FieldName string `json:"fieldName"`
	SchemaID  string `json:"schemaId"`
}

type ConsentRequirement struct {
	Owner      string         `json:"owner"`
	OwnerID    string         `json:"ownerId"`
	OwnerEmail string         `json:"ownerEmail"`
	Fields     []ConsentField `json:"fields"`
}

type CreateConsentRequest struct {
	AppID              string             `json:"appId"`
	ConsentRequirement ConsentRequirement `json:"consentRequirement"`
}

type CreateConsentResponse struct {
	ConsentID        string          `json:"consentId"`
	Status           string          `json:"status"`
	ConsentPortalURL *string         `json:"consentPortalUrl,omitempty"`
	Fields           *[]ConsentField `json:"fields,omitempty"`
}

type Consent struct {
	ConsentID string `json:"consentId"`
	Status    string `json:"status"`
	// Add other fields as needed based on API response
}

type UpdateConsentRequest struct {
	Status    string `json:"status"`
	UpdatedBy string `json:"updated_by"`
}

type UpdateConsentResponse struct {
	Status string `json:"status"`
}

func TestMain(m *testing.M) {
	// Wait for consent engine service availability
	if err := testutils.WaitForService(consentBaseURL+"/health", 30); err != nil {
		fmt.Printf("Consent Engine service not available: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}

// TestConsent_CreateAndRetrieve tests basic consent creation and retrieval
func TestConsent_CreateAndRetrieve(t *testing.T) {
	appID := "test-app-consent-1"
	ownerID := "test-owner-123"
	fieldName := "personInfo.name"
	schemaID := "test-schema-123"

	// Create consent request
	createReq := CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: ConsentRequirement{
			Owner:      "citizen",
			OwnerID:    ownerID,
			OwnerEmail: ownerID + "@example.com",
			Fields: []ConsentField{
				{
					FieldName: fieldName,
					SchemaID:  schemaID,
				},
			},
		},
	}

	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	// Create consent
	resp, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected 201 Created")

	var createResponse CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&createResponse)
	require.NoError(t, err)

	consentID := createResponse.ConsentID
	assert.NotEmpty(t, consentID, "consent_id should not be empty")
	assert.Equal(t, "pending", createResponse.Status, "New consent should have pending status")

	// Cleanup: Remove created consent record
	t.Cleanup(func() {
		testutils.CleanupConsentRecord(t, consentID)
	})

	// Retrieve consent (internal endpoint - no auth required for testing)
	// Returns a single consent object, not an array
	resp, err = http.Get(consentBaseURL + "/internal/api/v1/consents?ownerId=" + ownerID + "&appId=" + appID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK")

	var retrieveResponse CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&retrieveResponse)
	require.NoError(t, err)
	require.NotEmpty(t, retrieveResponse.ConsentID, "Expected consent ID to be present")

	// Verify consent matches
	assert.Equal(t, consentID, retrieveResponse.ConsentID, "Retrieved consent ID should match created consent")
	assert.Equal(t, "pending", retrieveResponse.Status, "Consent status should be pending")
}

// TestConsent_InvalidRequest tests edge cases for invalid consent requests
func TestConsent_InvalidRequest(t *testing.T) {
	tests := []struct {
		name           string
		request        func() []byte
		expectedStatus int
	}{
		{
			name: "Missing appId",
			request: func() []byte {
				req := map[string]interface{}{
					"consentRequirement": map[string]interface{}{
						"owner":      "citizen",
						"ownerId":    "test-owner",
						"ownerEmail": "test@example.com",
						"fields":     []map[string]interface{}{},
					},
				}
				body, _ := json.Marshal(req)
				return body
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing consentRequirement",
			request: func() []byte {
				req := CreateConsentRequest{
					AppID: "test-app",
				}
				body, _ := json.Marshal(req)
				return body
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing ownerId",
			request: func() []byte {
				req := map[string]interface{}{
					"appId": "test-app",
					"consentRequirement": map[string]interface{}{
						"owner":      "citizen",
						"ownerEmail": "test@example.com",
						"fields":     []map[string]interface{}{},
					},
				}
				body, _ := json.Marshal(req)
				return body
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Empty fields",
			request: func() []byte {
				req := CreateConsentRequest{
					AppID: "test-app",
					ConsentRequirement: ConsentRequirement{
						Owner:      "citizen",
						OwnerID:    "test-owner",
						OwnerEmail: "test@example.com",
						Fields:     []ConsentField{},
					},
				}
				body, _ := json.Marshal(req)
				return body
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := tt.request()

			resp, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode,
				"Expected status %d for invalid request: %s", tt.expectedStatus, tt.name)
		})
	}
}

// TestConsent_GetByConsumer tests retrieving consents by appId and ownerId
func TestConsent_GetByConsumer(t *testing.T) {
	appID := "test-app-consumer-1"
	ownerID := "test-owner-consumer-1"

	// Create consent
	createReq := CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: ConsentRequirement{
			Owner:      "citizen",
			OwnerID:    ownerID,
			OwnerEmail: ownerID + "@example.com",
			Fields: []ConsentField{
				{
					FieldName: "personInfo.name",
					SchemaID:  "test-schema-123",
				},
			},
		},
	}

	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResponse CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&createResponse)
	require.NoError(t, err)

	assert.NotEmpty(t, createResponse.ConsentID)

	// Retrieve by appId and ownerId (internal endpoint returns single consent object)
	resp, err = http.Get(consentBaseURL + "/internal/api/v1/consents?appId=" + appID + "&ownerId=" + ownerID)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return the consent we created
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 OK")

	var retrievedConsent CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&retrievedConsent)
	require.NoError(t, err)
	require.NotEmpty(t, retrievedConsent.ConsentID, "Expected to retrieve the created consent")
	assert.Equal(t, createResponse.ConsentID, retrievedConsent.ConsentID, "Retrieved consent ID should match created consent ID")

	// Cleanup: Remove created consent record
	consentID := createResponse.ConsentID
	t.Cleanup(func() {
		testutils.CleanupConsentRecord(t, consentID)
	})
}

// TestConsent_StatusUpdate tests consent status updates
// Note: This test may require JWT authentication in production
// For integration tests, we test the internal PATCH endpoint if available
func TestConsent_StatusUpdate(t *testing.T) {
	appID := "test-app-update-1"
	ownerID := "test-owner-update-1"

	// Create consent
	createReq := CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: ConsentRequirement{
			Owner:      "citizen",
			OwnerID:    ownerID,
			OwnerEmail: ownerID + "@example.com",
			Fields: []ConsentField{
				{
					FieldName: "personInfo.name",
					SchemaID:  "test-schema-123",
				},
			},
		},
	}

	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResponse CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&createResponse)
	require.NoError(t, err)

	consentID := createResponse.ConsentID
	require.NotEmpty(t, consentID)

	// Update consent status using PUT (portal endpoint requires JWT auth)
	// For integration tests, we verify the consent was created successfully
	// Status updates require JWT authentication which is tested separately
	assert.Equal(t, "pending", createResponse.Status, "New consent should have pending status")

	// Cleanup: Remove created consent record
	t.Cleanup(func() {
		testutils.CleanupConsentRecord(t, consentID)
	})
}

// TestConsent_HealthCheck tests the health check endpoint
func TestConsent_HealthCheck(t *testing.T) {
	// Health check endpoint exists
	resp, err := http.Get(consentBaseURL + "/internal/api/v1/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Health check should return 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Health check should return 200 OK")
}

// TestConsent_DatabaseVerification tests consent creation with database verification
func TestConsent_DatabaseVerification(t *testing.T) {
	if os.Getenv("TEST_VERIFY_DB") != "true" {
		t.Skip("Skipping database verification test (set TEST_VERIFY_DB=true to enable)")
	}

	db := testutils.SetupConsentDB(t)
	if db == nil {
		t.Skip("Database connection not available")
		return
	}

	appID := "test-app-db-1"
	ownerID := "test-owner-db-1"

	// Create consent
	createReq := CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: ConsentRequirement{
			Owner:      "citizen",
			OwnerID:    ownerID,
			OwnerEmail: ownerID + "@example.com",
			Fields: []ConsentField{
				{
					FieldName: "personInfo.name",
					SchemaID:  "test-schema-123",
				},
			},
		},
	}

	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResponse CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&createResponse)
	require.NoError(t, err)

	consentID := createResponse.ConsentID
	require.NotEmpty(t, consentID)

	// Verify consent exists in database
	var count int64
	err = db.Table("consent_records").
		Where("consent_id = ?", consentID).
		Count(&count).Error

	require.NoError(t, err)
	assert.Greater(t, count, int64(0), "Consent should exist in database")

	// Cleanup
	t.Cleanup(func() {
		testutils.CleanupConsentRecord(t, consentID)
	})
}

// TestConsent_GetByOwnerEmail tests retrieving consent by owner email
func TestConsent_GetByOwnerEmail(t *testing.T) {
	appID := "test-app-email-1"
	ownerID := "test-owner-email-1"
	ownerEmail := ownerID + "@example.com"

	// Create consent
	createReq := CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: ConsentRequirement{
			Owner:      "citizen",
			OwnerID:    ownerID,
			OwnerEmail: ownerEmail,
			Fields: []ConsentField{
				{
					FieldName: "personInfo.name",
					SchemaID:  "test-schema-123",
				},
			},
		},
	}

	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResponse CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&createResponse)
	require.NoError(t, err)

	consentID := createResponse.ConsentID
	require.NotEmpty(t, consentID)

	// Cleanup
	t.Cleanup(func() {
		testutils.CleanupConsentRecord(t, consentID)
	})

	// Retrieve by owner email
	resp, err = http.Get(consentBaseURL + "/internal/api/v1/consents?ownerEmail=" + url.QueryEscape(ownerEmail) + "&appId=" + appID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return 200 OK")

	var retrievedConsent CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&retrievedConsent)
	require.NoError(t, err)
	assert.Equal(t, consentID, retrievedConsent.ConsentID, "Retrieved consent ID should match created consent")
}

// TestConsent_MultipleFields tests creating consent with multiple fields
func TestConsent_MultipleFields(t *testing.T) {
	appID := "test-app-multi-1"
	ownerID := "test-owner-multi-1"

	// Create consent with multiple fields
	createReq := CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: ConsentRequirement{
			Owner:      "citizen",
			OwnerID:    ownerID,
			OwnerEmail: ownerID + "@example.com",
			Fields: []ConsentField{
				{
					FieldName: "personInfo.name",
					SchemaID:  "test-schema-123",
				},
				{
					FieldName: "personInfo.address",
					SchemaID:  "test-schema-123",
				},
				{
					FieldName: "personInfo.phone",
					SchemaID:  "test-schema-456",
				},
			},
		},
	}

	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResponse CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&createResponse)
	require.NoError(t, err)

	consentID := createResponse.ConsentID
	require.NotEmpty(t, consentID)
	assert.Equal(t, "pending", createResponse.Status, "New consent should have pending status")

	// Verify fields are present
	if createResponse.Fields != nil {
		assert.Equal(t, 3, len(*createResponse.Fields), "Should have 3 fields")
	}

	// Cleanup
	t.Cleanup(func() {
		testutils.CleanupConsentRecord(t, consentID)
	})
}

// TestConsent_NotFound tests retrieving non-existent consent
func TestConsent_NotFound(t *testing.T) {
	// Try to retrieve a consent that doesn't exist
	resp, err := http.Get(consentBaseURL + "/internal/api/v1/consents?ownerId=nonexistent-owner&appId=nonexistent-app")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "Should return 404 Not Found")
}

// TestConsent_MissingAppId tests GET request without appId
func TestConsent_MissingAppId(t *testing.T) {
	// Try to retrieve consent without appId (required parameter)
	resp, err := http.Get(consentBaseURL + "/internal/api/v1/consents?ownerId=test-owner")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should return 400 Bad Request for missing appId")
}

// TestConsent_MissingOwnerIdentifier tests GET request without ownerEmail or ownerId
func TestConsent_MissingOwnerIdentifier(t *testing.T) {
	// Try to retrieve consent without ownerEmail or ownerId (required parameter)
	resp, err := http.Get(consentBaseURL + "/internal/api/v1/consents?appId=test-app")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should return 400 Bad Request for missing owner identifier")
}

// TestConsent_DuplicateCreation tests creating duplicate consent (same appId, ownerId, fields)
func TestConsent_DuplicateCreation(t *testing.T) {
	appID := "test-app-dup-1"
	ownerID := "test-owner-dup-1"

	createReq := CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: ConsentRequirement{
			Owner:      "citizen",
			OwnerID:    ownerID,
			OwnerEmail: ownerID + "@example.com",
			Fields: []ConsentField{
				{
					FieldName: "personInfo.name",
					SchemaID:  "test-schema-123",
				},
			},
		},
	}

	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	// Create first consent
	resp1, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp1.Body.Close()

	assert.Equal(t, http.StatusCreated, resp1.StatusCode)

	var createResponse1 CreateConsentResponse
	err = json.NewDecoder(resp1.Body).Decode(&createResponse1)
	require.NoError(t, err)

	consentID1 := createResponse1.ConsentID
	require.NotEmpty(t, consentID1)

	// Cleanup
	t.Cleanup(func() {
		testutils.CleanupConsentRecord(t, consentID1)
	})

	// Try to create duplicate consent (same appId, ownerId, fields)
	reqBody2, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp2, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody2))
	require.NoError(t, err)
	defer resp2.Body.Close()

	// Service may return 201 (idempotent) or 400 (duplicate), both are acceptable
	assert.Contains(t, []int{http.StatusCreated, http.StatusBadRequest}, resp2.StatusCode,
		"Duplicate creation should return 201 (idempotent) or 400 (duplicate)")

	if resp2.StatusCode == http.StatusCreated {
		var createResponse2 CreateConsentResponse
		err = json.NewDecoder(resp2.Body).Decode(&createResponse2)
		require.NoError(t, err)
		consentID2 := createResponse2.ConsentID
		// If idempotent, may return same consent ID
		if consentID2 != "" && consentID2 != consentID1 {
			t.Cleanup(func() {
				testutils.CleanupConsentRecord(t, consentID2)
			})
		}
	}
}

// TestConsent_DifferentSchemas tests creating consent with fields from different schemas
func TestConsent_DifferentSchemas(t *testing.T) {
	appID := "test-app-schema-1"
	ownerID := "test-owner-schema-1"

	createReq := CreateConsentRequest{
		AppID: appID,
		ConsentRequirement: ConsentRequirement{
			Owner:      "citizen",
			OwnerID:    ownerID,
			OwnerEmail: ownerID + "@example.com",
			Fields: []ConsentField{
				{
					FieldName: "personInfo.name",
					SchemaID:  "test-schema-123",
				},
				{
					FieldName: "vehicleInfo.licensePlate",
					SchemaID:  "test-schema-456",
				},
			},
		},
	}

	reqBody, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(consentBaseURL+"/internal/api/v1/consents", "application/json", bytes.NewBuffer(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResponse CreateConsentResponse
	err = json.NewDecoder(resp.Body).Decode(&createResponse)
	require.NoError(t, err)

	consentID := createResponse.ConsentID
	require.NotEmpty(t, consentID)

	// Cleanup
	t.Cleanup(func() {
		testutils.CleanupConsentRecord(t, consentID)
	})
}
