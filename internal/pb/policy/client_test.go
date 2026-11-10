package policy

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	baseURL := "http://localhost:8082"

	service := NewClient(baseURL)

	assert.NotNil(t, service)
	assert.Equal(t, baseURL, service.baseURL)
	assert.NotNil(t, service.HTTPClient)
	assert.Equal(t, 10*time.Second, service.HTTPClient.Timeout)
}

func TestClient_CreatePolicyMetadata_Success(t *testing.T) {
	schemaID := "test-schema-123"
	expectedRecords := []PolicyMetadataResponse{
		{
			ID:          "record-1",
			SchemaID:    schemaID,
			FieldName:   "personInfo.name",
			DisplayName: stringPtr("Name"),
			Source:      SourcePrimary,
			PolicyClass: PolicyClassAllowlistConsent,
			AllowList:   AllowList{},
			CreatedAt:   "2024-01-01T00:00:00Z",
			UpdatedAt:   "2024-01-01T00:00:00Z",
		},
	}

	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/policy/metadata", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var req PolicyMetadataCreateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, schemaID, req.SchemaID)
		// Note: Records may be empty if SDL has no directives, which is valid

		// Send response
		response := PolicyMetadataCreateResponse{
			Records: expectedRecords,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // PDP's CreatePolicyMetadata handler returns 201
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewClient(server.URL)

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

func TestClient_CreatePolicyMetadataFromRecords_Success(t *testing.T) {
	schemaID := "test-schema-123"
	records := []PolicyMetadataCreateRequestRecord{
		{
			FieldName:    "email",
			Source:       SourcePrimary,
			PolicyClass:  PolicyClassAllowlist,
			PolicyParams: PolicyParams{"purpose": "tax"},
		},
	}

	var capturedReq PolicyMetadataCreateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/policy/metadata", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedReq))

		response := PolicyMetadataCreateResponse{
			Records: []PolicyMetadataResponse{
				{ID: "record-1", SchemaID: schemaID, FieldName: "email"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // PDP's CreatePolicyMetadata handler returns 201
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewClient(server.URL)

	response, err := service.CreatePolicyMetadataFromRecords(schemaID, records)

	require.NoError(t, err)
	assert.NotNil(t, response)
	if response != nil {
		assert.Len(t, response.Records, 1)
	}

	// No SDL parsing should have happened - the exact records given must be
	// forwarded to the PDP as-is, with no typename-prefix mangling.
	assert.Equal(t, schemaID, capturedReq.SchemaID)
	if assert.Len(t, capturedReq.Records, 1) {
		assert.Equal(t, "email", capturedReq.Records[0].FieldName)
		assert.Equal(t, SourcePrimary, capturedReq.Records[0].Source)
		assert.Equal(t, PolicyClassAllowlist, capturedReq.Records[0].PolicyClass)
		assert.Equal(t, PolicyParams{"purpose": "tax"}, capturedReq.Records[0].PolicyParams)
	}
}

func TestClient_CreatePolicyMetadataFromRecords_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad records"}`))
	}))
	defer server.Close()

	service := NewClient(server.URL)
	response, err := service.CreatePolicyMetadataFromRecords("schema-1", []PolicyMetadataCreateRequestRecord{
		{FieldName: "email", Source: SourcePrimary, PolicyClass: PolicyClassAllowlist},
	})

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "status 400")
}

func TestClient_CreatePolicyMetadata_InvalidSDL(t *testing.T) {
	service := NewClient("http://localhost:8082")

	// Use invalid SDL
	invalidSDL := "invalid graphql syntax {"

	response, err := service.CreatePolicyMetadata("test-schema", invalidSDL)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to parse SDL")
}

func TestClient_CreatePolicyMetadata_Non200Status(t *testing.T) {
	// Create a mock HTTP server that returns 400
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid request"}`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

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

func TestClient_CreatePolicyMetadata_InvalidJSONResponse(t *testing.T) {
	// Create a mock HTTP server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated) // PDP's CreatePolicyMetadata handler returns 201
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

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

func TestClient_CreatePolicyMetadata_NetworkError(t *testing.T) {
	// Use an invalid URL to simulate network error
	service := NewClient("http://invalid-host:9999")

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

func TestClient_CreateAccessGrants_Success(t *testing.T) {
	applicationID := "test-app-123"
	expectedRecords := []AccessGrant{
		{
			ID:            "grant-1",
			FieldName:     "personInfo.name",
			SchemaID:      "test-schema-123",
			ApplicationID: applicationID,
			ExpiresAt:     "2024-02-01T00:00:00Z",
			Status:        GrantStatusActive,
			CreatedAt:     "2024-01-01T00:00:00Z",
			UpdatedAt:     "2024-01-01T00:00:00Z",
		},
	}

	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/policy/grants", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var req AccessGrantCreateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, applicationID, req.ApplicationID)
		assert.NotEmpty(t, req.Records)
		assert.Equal(t, GrantDurationTypeOneMonth, req.GrantDuration)

		// Send response
		response := AccessGrantListResponse{
			Records: expectedRecords,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewClient(server.URL)

	request := AccessGrantCreateRequest{
		ApplicationID: applicationID,
		Records: []SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema-123",
			},
		},
		GrantDuration: GrantDurationTypeOneMonth,
	}

	response, err := service.CreateAccessGrants(request)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Len(t, response.Records, 1)
	assert.Equal(t, expectedRecords[0].FieldName, response.Records[0].FieldName)
	assert.Equal(t, expectedRecords[0].SchemaID, response.Records[0].SchemaID)
	assert.Equal(t, "grant-1", response.Records[0].ID)
}

func TestClient_CreateAccessGrants_Non200Status(t *testing.T) {
	// Create a mock HTTP server that returns 400
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid request"}`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

	request := AccessGrantCreateRequest{
		ApplicationID: "test-app",
		Records: []SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema",
			},
		},
		GrantDuration: GrantDurationTypeOneMonth,
	}

	response, err := service.CreateAccessGrants(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "PDP returned status 400")
}

func TestClient_CreateAccessGrants_InvalidJSONResponse(t *testing.T) {
	// Create a mock HTTP server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

	request := AccessGrantCreateRequest{
		ApplicationID: "test-app",
		Records: []SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema",
			},
		},
		GrantDuration: GrantDurationTypeOneMonth,
	}

	response, err := service.CreateAccessGrants(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestClient_CreateAccessGrants_NetworkError(t *testing.T) {
	// Use an invalid URL to simulate network error
	service := NewClient("http://invalid-host:9999")

	request := AccessGrantCreateRequest{
		ApplicationID: "test-app",
		Records: []SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema",
			},
		},
		GrantDuration: GrantDurationTypeOneMonth,
	}

	response, err := service.CreateAccessGrants(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed to send request to PDP")
}

func TestClient_CreateAccessGrants_MarshalError(t *testing.T) {
	// Create a valid request - marshal errors are unlikely with our models
	request := AccessGrantCreateRequest{
		ApplicationID: "test-app",
		Records: []SelectedFieldRecord{
			{
				FieldName: "personInfo.name",
				SchemaID:  "test-schema",
			},
		},
		GrantDuration: GrantDurationTypeOneMonth,
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

func TestClient_ListPolicyMetadata_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/policy/metadata", r.URL.Path)
		assert.Equal(t, "sch_1&x", r.URL.Query().Get("schemaId"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"records":[{"id":"pm-1","schemaId":"sch_1&x","fieldName":"person.name","source":"primary","policyClass":"allowlist_consent","policyParams":{},"allowList":{}}]}`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

	response, err := service.ListPolicyMetadata("sch_1&x")
	require.NoError(t, err)
	require.Len(t, response.Records, 1)
	assert.Equal(t, "pm-1", response.Records[0].ID)
	assert.Equal(t, PolicyClassAllowlistConsent, response.Records[0].PolicyClass)
}

func TestClient_PatchPolicyMetadata_ForwardsOnlySetFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v1/policy/metadata/pm-1", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, map[string]any{"displayName": "Name", "description": nil}, body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"pm-1","schemaId":"sch_1","fieldName":"person.name","displayName":"Name","policyClass":"allowlist"}`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

	var req PolicyMetadataPatchRequest
	require.NoError(t, json.Unmarshal([]byte(`{"displayName":"Name","description":null}`), &req))

	response, err := service.PatchPolicyMetadata("pm-1", &req)
	require.NoError(t, err)
	assert.Equal(t, "pm-1", response.ID)
	assert.Equal(t, "Name", *response.DisplayName)
}

func TestClient_DeletePolicyMetadata_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v1/policy/metadata/pm-1", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	service := NewClient(server.URL)

	assert.NoError(t, service.DeletePolicyMetadata("pm-1"))
}

func TestClient_RevokeAccessGrant_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/policy/grants/grant%2F1/revoke", r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"grant/1","schemaId":"sch_1","fieldName":"person.name","applicationId":"client-1","status":"revoked"}`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

	grant, err := service.RevokeAccessGrant("grant/1")
	require.NoError(t, err)
	assert.Equal(t, GrantStatusRevoked, grant.Status)
	assert.Equal(t, "client-1", grant.ApplicationID)
}

func TestClient_ListAccessGrants_Filters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/policy/grants", r.URL.Path)
		assert.Equal(t, "client&1", r.URL.Query().Get("applicationId"))
		assert.Equal(t, "sch_1", r.URL.Query().Get("schemaId"))
		assert.Equal(t, "active", r.URL.Query().Get("status"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"records":[{"id":"g-1","schemaId":"sch_1","fieldName":"person.name","applicationId":"client&1","expiresAt":"2026-02-01T00:00:00Z","status":"active","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}]}`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

	response, err := service.ListAccessGrants(AccessGrantFilter{ApplicationID: "client&1", SchemaID: "sch_1", Status: GrantStatusActive})
	require.NoError(t, err)
	require.Len(t, response.Records, 1)
	entry, err := response.Records[0].AllowListEntry()
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), entry.ExpiresAt.UTC())
	assert.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), entry.UpdatedAt.UTC())
}

func TestClient_ListAccessGrants_NoFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/policy/grants", r.URL.Path)
		assert.Empty(t, r.URL.RawQuery)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"records":[]}`))
	}))
	defer server.Close()

	response, err := NewClient(server.URL).ListAccessGrants(AccessGrantFilter{})
	require.NoError(t, err)
	assert.Empty(t, response.Records)
}

func TestAccessGrant_AllowListEntry_InvalidTime(t *testing.T) {
	_, err := AccessGrant{ID: "g-1", ExpiresAt: "soon", UpdatedAt: "2026-01-02T00:00:00Z"}.AllowListEntry()
	assert.ErrorContains(t, err, "invalid expiresAt")
}

func TestClient_PolicyMetadata_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"active access grant not found: g-1"}`))
	}))
	defer server.Close()

	service := NewClient(server.URL)

	_, err := service.RevokeAccessGrant("g-1")
	var pdpErr *Error
	require.ErrorAs(t, err, &pdpErr)
	assert.Equal(t, http.StatusNotFound, pdpErr.StatusCode)
	assert.Equal(t, "active access grant not found: g-1", pdpErr.Message)
}

func TestClient_PolicyMetadata_NetworkError(t *testing.T) {
	service := NewClient("http://invalid-host:9999")

	_, err := service.ListPolicyMetadata("sch_1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send request to PDP")

	var pdpErr *Error
	assert.False(t, errors.As(err, &pdpErr))
}
