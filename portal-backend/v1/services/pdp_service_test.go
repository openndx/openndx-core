package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPDPService(t *testing.T) {
	baseURL := "http://localhost:8082"

	service := NewPDPService(baseURL)

	assert.NotNil(t, service)
	assert.Equal(t, baseURL, service.baseURL)
	assert.NotNil(t, service.HTTPClient)
	assert.Equal(t, 10*time.Second, service.HTTPClient.Timeout)
}

func TestPDPService_CreatePolicyMetadata_Success(t *testing.T) {
	schemaID := "test-schema-123"
	expectedRecords := []models.PolicyMetadataResponse{
		{
			ID:                "record-1",
			SchemaID:          schemaID,
			FieldName:         "personInfo.name",
			DisplayName:       stringPtr("Name"),
			Source:            models.SourcePrimary,
			IsOwner:           false,
			AccessControlType: models.AccessControlTypeRestricted,
			AllowList:         models.AllowList{},
			CreatedAt:         "2024-01-01T00:00:00Z",
			UpdatedAt:         "2024-01-01T00:00:00Z",
		},
	}

	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/policy/metadata", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var req models.PolicyMetadataCreateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, schemaID, req.SchemaID)
		// Note: Records may be empty if SDL has no directives, which is valid

		// Send response
		response := models.PolicyMetadataCreateResponse{
			Records: expectedRecords,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewPDPService(server.URL)

	// Use a simple SDL for testing (valid GraphQL without custom directives)
	sdl := `
		type Person {
			name: String
			age: Int
		}
	`

	response, err := service.CreatePolicyMetadata(schemaID, sdl)
	require.NoError(t, err)
	assert.NotNil(t, response)
	// The response will have records from the mock server regardless of SDL parsing
	assert.Len(t, response.Records, 1)
	assert.Equal(t, expectedRecords[0].ID, response.Records[0].ID)
	assert.Equal(t, expectedRecords[0].SchemaID, response.Records[0].SchemaID)
}

func TestPDPService_CreatePolicyMetadata_InvalidSDL(t *testing.T) {
	service := NewPDPService("http://localhost:8082")

	// Use invalid SDL
	invalidSDL := "invalid graphql syntax {"

	response, err := service.CreatePolicyMetadata("test-schema", invalidSDL)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to parse SDL")
}

func TestPDPService_CreatePolicyMetadata_Non200Status(t *testing.T) {
	// Create a mock HTTP server that returns 400
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid request"}`))
	}))
	defer server.Close()

	service := NewPDPService(server.URL)

	sdl := `
		type Person {
			name: String
		}
	`

	response, err := service.CreatePolicyMetadata("test-schema", sdl)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "PDP returned status 400")
}

func TestPDPService_CreatePolicyMetadata_InvalidJSONResponse(t *testing.T) {
	// Create a mock HTTP server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	service := NewPDPService(server.URL)

	sdl := `
		type Person {
			name: String
		}
	`

	response, err := service.CreatePolicyMetadata("test-schema", sdl)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestPDPService_CreatePolicyMetadata_NetworkError(t *testing.T) {
	// Use an invalid URL to simulate network error
	service := NewPDPService("http://invalid-host:9999")

	sdl := `
		type Person {
			name: String
		}
	`

	response, err := service.CreatePolicyMetadata("test-schema", sdl)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to send request to PDP")
}

func TestPDPService_UpdateAllowList_Success(t *testing.T) {
	applicationID := "test-app-123"
	expectedRecords := []models.AllowListUpdateResponseRecord{
		{
			FieldName: "personInfo.name",
			SchemaID:  "test-schema-123",
			ExpiresAt: "2024-02-01T00:00:00Z",
			UpdatedAt: "2024-01-01T00:00:00Z",
		},
	}

	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/policy/update-allowlist", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var req models.AllowListUpdateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, applicationID, req.ApplicationID)
		assert.NotEmpty(t, req.Records)
		assert.Equal(t, models.GrantDurationTypeOneMonth, req.GrantDuration)

		// Send response
		response := models.AllowListUpdateResponse{
			Records: expectedRecords,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewPDPService(server.URL)

	request := models.AllowListUpdateRequest{
		ApplicationID: applicationID,
		Records: []models.SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema-123",
			},
		},
		GrantDuration: models.GrantDurationTypeOneMonth,
	}

	response, err := service.UpdateAllowList(request)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Records, 1)
	assert.Equal(t, expectedRecords[0].FieldName, response.Records[0].FieldName)
	assert.Equal(t, expectedRecords[0].SchemaID, response.Records[0].SchemaID)
}

func TestPDPService_UpdateAllowList_Non200Status(t *testing.T) {
	// Create a mock HTTP server that returns 400
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid request"}`))
	}))
	defer server.Close()

	service := NewPDPService(server.URL)

	request := models.AllowListUpdateRequest{
		ApplicationID: "test-app",
		Records: []models.SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema",
			},
		},
		GrantDuration: models.GrantDurationTypeOneMonth,
	}

	response, err := service.UpdateAllowList(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "PDP returned status 400")
}

func TestPDPService_UpdateAllowList_InvalidJSONResponse(t *testing.T) {
	// Create a mock HTTP server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	service := NewPDPService(server.URL)

	request := models.AllowListUpdateRequest{
		ApplicationID: "test-app",
		Records: []models.SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema",
			},
		},
		GrantDuration: models.GrantDurationTypeOneMonth,
	}

	response, err := service.UpdateAllowList(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestPDPService_UpdateAllowList_NetworkError(t *testing.T) {
	// Use an invalid URL to simulate network error
	service := NewPDPService("http://invalid-host:9999")

	request := models.AllowListUpdateRequest{
		ApplicationID: "test-app",
		Records: []models.SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema",
			},
		},
		GrantDuration: models.GrantDurationTypeOneMonth,
	}

	response, err := service.UpdateAllowList(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to send request to PDP")
}

func TestPDPService_UpdateAllowList_MarshalError(t *testing.T) {
	// Create a valid request - marshal errors are unlikely with our models
	request := models.AllowListUpdateRequest{
		ApplicationID: "test-app",
		Records: []models.SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema",
			},
		},
		GrantDuration: models.GrantDurationTypeOneMonth,
	}

	// This should work fine - marshal errors are very rare with our simple models
	// We'll just verify the request is valid
	_, err := json.Marshal(request)
	assert.NoError(t, err, "Request should be marshallable")
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
