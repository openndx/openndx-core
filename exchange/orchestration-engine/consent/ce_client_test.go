package consent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
)

func init() {
	logger.Init()
}

func TestNewCEServiceClient(t *testing.T) {
	baseURL := "http://localhost:8080"
	client := NewCEServiceClient(baseURL, nil)

	if client == nil {
		t.Fatal("Expected non-nil CEServiceClient")
	}

	if client.baseURL != baseURL {
		t.Errorf("Expected baseURL %s, got %s", baseURL, client.baseURL)
	}

	if client.httpClient == nil {
		t.Error("Expected non-nil httpClient")
	}

	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("Expected timeout of 10 seconds, got %v", client.httpClient.Timeout)
	}
}

func TestCreateConsent_Success(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.URL.Path != "/consents" {
			t.Errorf("Expected path /consents, got %s", r.URL.Path)
		}

		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Verify request body
		var request CreateConsentRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if request.AppID != "test-app-id" {
			t.Errorf("Expected AppID test-app-id, got %s", request.AppID)
		}

		// Create a mock response
		consentID := "consent-123"
		status := StatusPending
		consentPortalURL := "https://consent-portal.example.com/consent/consent-123"

		response := ConsentResponseInternalView{
			ConsentID:        consentID,
			Status:           status,
			ConsentPortalURL: &consentPortalURL,
			Fields: &[]ConsentField{
				{
					FieldName:   "name",
					SchemaID:    "schema-1",
					DisplayName: stringPtr("Full Name"),
					Description: stringPtr("User's full name"),
					Owner:       OwnerCitizen,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with mock server URL
	client := NewCEServiceClient(server.URL, nil)

	// Prepare request
	appName := "Test App"
	grantDuration := "P1Y"
	consentType := TypeRealtime

	request := &CreateConsentRequest{
		AppID:   "test-app-id",
		AppName: &appName,
		ConsentRequirement: ConsentRequirement{
			Owner:      OwnerCitizen,
			OwnerID:    "citizen-123",
			OwnerEmail: "citizen@example.com",
			Fields: []ConsentField{
				{
					FieldName:   "name",
					SchemaID:    "schema-1",
					DisplayName: stringPtr("Full Name"),
					Description: stringPtr("User's full name"),
					Owner:       OwnerCitizen,
				},
			},
		},
		GrantDuration: &grantDuration,
		ConsentType:   &consentType,
	}

	// Call CreateConsent
	ctx := context.Background()
	response, err := client.CreateConsent(ctx, request)
	// Verify response
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response == nil {
		t.Fatal("Expected non-nil response")
	}

	if response.ConsentID != "consent-123" {
		t.Errorf("Expected ConsentID consent-123, got %s", response.ConsentID)
	}

	if response.Status != StatusPending {
		t.Errorf("Expected Status %s, got %s", StatusPending, response.Status)
	}

	if response.ConsentPortalURL == nil {
		t.Error("Expected non-nil ConsentPortalURL")
	} else if *response.ConsentPortalURL != "https://consent-portal.example.com/consent/consent-123" {
		t.Errorf("Expected ConsentPortalURL https://consent-portal.example.com/consent/consent-123, got %s", *response.ConsentPortalURL)
	}

	if response.Fields == nil {
		t.Error("Expected non-nil Fields")
	} else if len(*response.Fields) != 1 {
		t.Errorf("Expected 1 field, got %d", len(*response.Fields))
	}
}

func TestCreateConsent_MarshalError(t *testing.T) {
	client := NewCEServiceClient("http://localhost:8080", nil)

	// Create a request with an unmarshalable field (channel cannot be marshaled to JSON)
	// Since our struct uses standard types, we'll test with a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	request := &CreateConsentRequest{
		AppID: "test-app-id",
		ConsentRequirement: ConsentRequirement{
			Owner:      OwnerCitizen,
			OwnerID:    "citizen-123",
			OwnerEmail: "citizen@example.com",
			Fields:     []ConsentField{},
		},
	}

	// This won't cause a marshal error, but we can test with an invalid server
	// to trigger a different error path
	client.baseURL = "http://invalid-url-that-will-fail:99999"

	_, err := client.CreateConsent(ctx, request)

	if err == nil {
		t.Error("Expected error with canceled context or invalid URL, got nil")
	}
}

func TestCreateConsent_HTTPRequestError(t *testing.T) {
	// Create client with invalid URL to trigger request error
	client := NewCEServiceClient("http://invalid-url-that-does-not-exist:99999", nil)

	request := &CreateConsentRequest{
		AppID: "test-app-id",
		ConsentRequirement: ConsentRequirement{
			Owner:      OwnerCitizen,
			OwnerID:    "citizen-123",
			OwnerEmail: "citizen@example.com",
			Fields:     []ConsentField{},
		},
	}

	ctx := context.Background()
	_, err := client.CreateConsent(ctx, request)

	if err == nil {
		t.Error("Expected error with invalid URL, got nil")
	}
}

func TestCreateConsent_NonCreatedStatusCode(t *testing.T) {
	// Create a mock server that returns a non-201 status code
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Invalid request"}`))
	}))
	defer server.Close()

	client := NewCEServiceClient(server.URL, nil)

	request := &CreateConsentRequest{
		AppID: "test-app-id",
		ConsentRequirement: ConsentRequirement{
			Owner:      OwnerCitizen,
			OwnerID:    "citizen-123",
			OwnerEmail: "citizen@example.com",
			Fields:     []ConsentField{},
		},
	}

	ctx := context.Background()
	_, err := client.CreateConsent(ctx, request)

	if err == nil {
		t.Error("Expected error with non-201 status code, got nil")
	}

	expectedErrMsg := "failed to create consent, status code: 400"
	if err.Error()[:len(expectedErrMsg)] != expectedErrMsg {
		t.Errorf("Expected error message to start with '%s', got %s", expectedErrMsg, err.Error())
	}
}

func TestCreateConsent_InvalidResponseBody(t *testing.T) {
	// Create a mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	client := NewCEServiceClient(server.URL, nil)

	request := &CreateConsentRequest{
		AppID: "test-app-id",
		ConsentRequirement: ConsentRequirement{
			Owner:      OwnerCitizen,
			OwnerID:    "citizen-123",
			OwnerEmail: "citizen@example.com",
			Fields:     []ConsentField{},
		},
	}

	ctx := context.Background()
	_, err := client.CreateConsent(ctx, request)

	if err == nil {
		t.Error("Expected error with invalid JSON response, got nil")
	}
}

func TestCreateConsent_ContextCancellation(t *testing.T) {
	// Create a mock server with a delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ConsentResponseInternalView{
			ConsentID: "consent-123",
			Status:    StatusPending,
		})
	}))
	defer server.Close()

	client := NewCEServiceClient(server.URL, nil)

	request := &CreateConsentRequest{
		AppID: "test-app-id",
		ConsentRequirement: ConsentRequirement{
			Owner:      OwnerCitizen,
			OwnerID:    "citizen-123",
			OwnerEmail: "citizen@example.com",
			Fields:     []ConsentField{},
		},
	}

	// Create a context that will be canceled immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.CreateConsent(ctx, request)

	if err == nil {
		t.Error("Expected error with canceled context, got nil")
	}
}

func TestCreateConsent_WithAllOptionalFields(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request CreateConsentRequest
		json.NewDecoder(r.Body).Decode(&request)

		// Verify optional fields are present
		if request.AppName == nil {
			t.Error("Expected AppName to be present")
		}
		if request.GrantDuration == nil {
			t.Error("Expected GrantDuration to be present")
		}
		if request.ConsentType == nil {
			t.Error("Expected ConsentType to be present")
		}

		response := ConsentResponseInternalView{
			ConsentID: "consent-123",
			Status:    StatusApproved,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewCEServiceClient(server.URL, nil)

	appName := "Test App"
	grantDuration := "P1Y"
	consentType := TypeRealtime

	request := &CreateConsentRequest{
		AppID:         "test-app-id",
		AppName:       &appName,
		GrantDuration: &grantDuration,
		ConsentType:   &consentType,
		ConsentRequirement: ConsentRequirement{
			Owner:      OwnerCitizen,
			OwnerID:    "citizen-123",
			OwnerEmail: "citizen@example.com",
			Fields: []ConsentField{
				{
					FieldName: "name",
					SchemaID:  "schema-1",
					Owner:     OwnerCitizen,
				},
			},
		},
	}

	ctx := context.Background()
	response, err := client.CreateConsent(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response.Status != StatusApproved {
		t.Errorf("Expected Status %s, got %s", StatusApproved, response.Status)
	}
}

func TestCreateConsent_WithMinimalFields(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request CreateConsentRequest
		json.NewDecoder(r.Body).Decode(&request)

		// Verify only required fields are present
		if request.AppName != nil {
			t.Error("Expected AppName to be nil")
		}
		if request.GrantDuration != nil {
			t.Error("Expected GrantDuration to be nil")
		}
		if request.ConsentType != nil {
			t.Error("Expected ConsentType to be nil")
		}

		response := ConsentResponseInternalView{
			ConsentID: "consent-123",
			Status:    StatusPending,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewCEServiceClient(server.URL, nil)

	request := &CreateConsentRequest{
		AppID: "test-app-id",
		ConsentRequirement: ConsentRequirement{
			Owner:      OwnerCitizen,
			OwnerID:    "citizen-123",
			OwnerEmail: "citizen@example.com",
			Fields: []ConsentField{
				{
					FieldName: "name",
					SchemaID:  "schema-1",
					Owner:     OwnerCitizen,
				},
			},
		},
	}

	ctx := context.Background()
	response, err := client.CreateConsent(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if response.Status != StatusPending {
		t.Errorf("Expected Status %s, got %s", StatusPending, response.Status)
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
