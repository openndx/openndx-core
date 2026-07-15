package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/auth"
)

func init() {
	// Initialize logger for tests
	logger.Init()
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name               string
		serviceKey         string
		serviceUrl         string
		schemaID           string
		authConfig         *auth.AuthConfig
		expectOAuth2Config bool
	}{
		{
			name:               "creates provider with no auth",
			serviceKey:         "provider1",
			serviceUrl:         "http://example.com",
			schemaID:           "schema1",
			authConfig:         nil,
			expectOAuth2Config: false,
		},
		{
			name:       "creates provider with API key auth",
			serviceKey: "provider2",
			serviceUrl: "http://example.com",
			schemaID:   "schema2",
			authConfig: &auth.AuthConfig{
				Type:        auth.AuthTypeAPIKey,
				APIKeyName:  "X-API-Key",
				APIKeyValue: "test-key-123",
			},
			expectOAuth2Config: false,
		},
		{
			name:       "creates provider with OAuth2 auth",
			serviceKey: "provider3",
			serviceUrl: "http://example.com",
			schemaID:   "schema3",
			authConfig: &auth.AuthConfig{
				Type:         auth.AuthTypeOAuth2,
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				TokenURL:     "http://auth.example.com/token",
				Scopes:       []string{"read", "write"},
			},
			expectOAuth2Config: true,
		},
		{
			name:               "creates provider with empty values",
			serviceKey:         "",
			serviceUrl:         "",
			schemaID:           "",
			authConfig:         nil,
			expectOAuth2Config: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewProvider(tt.serviceKey, tt.serviceUrl, tt.schemaID, tt.authConfig)

			if provider == nil {
				t.Fatal("NewProvider returned nil")
			}

			if provider.ServiceKey != tt.serviceKey {
				t.Errorf("Expected service key %s, got %s", tt.serviceKey, provider.ServiceKey)
			}

			if provider.ServiceUrl != tt.serviceUrl {
				t.Errorf("Expected service URL %s, got %s", tt.serviceUrl, provider.ServiceUrl)
			}

			if provider.SchemaID != tt.schemaID {
				t.Errorf("Expected schema ID %s, got %s", tt.schemaID, provider.SchemaID)
			}

			if provider.Auth != tt.authConfig {
				t.Error("Auth config doesn't match expected value")
			}

			if provider.Client == nil {
				t.Error("Expected Client to be initialized, got nil")
			}

			if provider.Headers == nil {
				t.Error("Expected Headers map to be initialized, got nil")
			}

			if tt.expectOAuth2Config {
				if provider.OAuth2Config == nil {
					t.Error("Expected OAuth2Config to be set, got nil")
				} else {
					if provider.OAuth2Config.ClientID != tt.authConfig.ClientID {
						t.Errorf("Expected client ID %s, got %s", tt.authConfig.ClientID, provider.OAuth2Config.ClientID)
					}
					if provider.OAuth2Config.ClientSecret != tt.authConfig.ClientSecret {
						t.Errorf("Expected client secret %s, got %s", tt.authConfig.ClientSecret, provider.OAuth2Config.ClientSecret)
					}
					if provider.OAuth2Config.TokenURL != tt.authConfig.TokenURL {
						t.Errorf("Expected token URL %s, got %s", tt.authConfig.TokenURL, provider.OAuth2Config.TokenURL)
					}
					if len(provider.OAuth2Config.Scopes) != len(tt.authConfig.Scopes) {
						t.Errorf("Expected %d scopes, got %d", len(tt.authConfig.Scopes), len(provider.OAuth2Config.Scopes))
					}
				}
			} else {
				if provider.OAuth2Config != nil {
					t.Error("Expected OAuth2Config to be nil, got non-nil value")
				}
			}
		})
	}
}

func TestProvider_PerformRequest_NoAuth(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Verify Content-Type header
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Read and verify body
		body, _ := io.ReadAll(r.Body)
		expectedBody := `{"test":"data"}`
		if string(body) != expectedBody {
			t.Errorf("Expected body %s, got %s", expectedBody, string(body))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	provider := NewProvider("test-provider", server.URL, "schema1", nil)
	ctx := context.Background()

	resp, err := provider.PerformRequest(ctx, []byte(`{"test":"data"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	expectedResp := `{"success":true}`
	if string(respBody) != expectedResp {
		t.Errorf("Expected response %s, got %s", expectedResp, string(respBody))
	}
}

func TestProvider_PerformRequest_APIKeyAuth(t *testing.T) {
	apiKeyName := "X-API-Key"
	apiKeyValue := "test-api-key-123"

	// Create a test server that validates the API key
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key header
		if r.Header.Get(apiKeyName) != apiKeyValue {
			t.Errorf("Expected API key %s, got %s", apiKeyValue, r.Header.Get(apiKeyName))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"authenticated":true}`))
	}))
	defer server.Close()

	authConfig := &auth.AuthConfig{
		Type:        auth.AuthTypeAPIKey,
		APIKeyName:  apiKeyName,
		APIKeyValue: apiKeyValue,
	}

	provider := NewProvider("test-provider", server.URL, "schema1", authConfig)
	ctx := context.Background()

	resp, err := provider.PerformRequest(ctx, []byte(`{"test":"data"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestProvider_PerformRequest_OAuth2Auth_NilConfig(t *testing.T) {
	// Test OAuth2 auth with nil OAuth2Config (error case)
	authConfig := &auth.AuthConfig{
		Type: auth.AuthTypeOAuth2,
	}

	provider := NewProvider("test-provider", "http://example.com", "schema1", nil)
	provider.Auth = authConfig
	provider.OAuth2Config = nil // Explicitly set to nil

	ctx := context.Background()

	_, err := provider.PerformRequest(ctx, []byte(`{"test":"data"}`))
	if err == nil {
		t.Error("Expected error when OAuth2Config is nil, got nil")
	}

	if !strings.Contains(err.Error(), "OAuth2Config is nil") {
		t.Errorf("Expected error message to contain 'OAuth2Config is nil', got: %v", err)
	}
}

func TestProvider_PerformRequest_OAuth2Auth(t *testing.T) {
	// Create a test server that simulates OAuth2 token endpoint and resource endpoint
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"test-access-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	resourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header
		if r.Header.Get("Authorization") != "Bearer test-access-token" {
			t.Errorf("Expected Authorization header 'Bearer test-access-token', got %s", r.Header.Get("Authorization"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"oauth_authenticated":true}`))
	}))
	defer resourceServer.Close()

	authConfig := &auth.AuthConfig{
		Type:         auth.AuthTypeOAuth2,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		TokenURL:     tokenServer.URL,
		Scopes:       []string{"read", "write"},
	}

	provider := NewProvider("test-provider", resourceServer.URL, "schema1", authConfig)
	ctx := context.Background()

	resp, err := provider.PerformRequest(ctx, []byte(`{"test":"data"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestProvider_PerformRequest_ContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewProvider("test-provider", server.URL, "schema1", nil)

	// Create a context that will be cancelled immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := provider.PerformRequest(ctx, []byte(`{"test":"data"}`))
	if err == nil {
		t.Error("Expected error due to cancelled context, got nil")
	}
}

func TestProvider_PerformRequest_InvalidURL(t *testing.T) {
	// Test with invalid URL
	provider := NewProvider("test-provider", "://invalid-url", "schema1", nil)
	ctx := context.Background()

	_, err := provider.PerformRequest(ctx, []byte(`{"test":"data"}`))
	if err == nil {
		t.Error("Expected error with invalid URL, got nil")
	}
}

func TestProvider_PerformRequest_ServerError(t *testing.T) {
	// Create a server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	provider := NewProvider("test-provider", server.URL, "schema1", nil)
	ctx := context.Background()

	resp, err := provider.PerformRequest(ctx, []byte(`{"test":"data"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", resp.StatusCode)
	}
}

func TestProvider_PerformRequest_EmptyBody(t *testing.T) {
	// Test with empty request body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("Expected empty body, got %s", string(body))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	provider := NewProvider("test-provider", server.URL, "schema1", nil)
	ctx := context.Background()

	resp, err := provider.PerformRequest(ctx, []byte{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestProvider_PerformRequest_LargePayload(t *testing.T) {
	// Test with large payload
	largePayload := make([]byte, 1024*1024) // 1MB
	for i := range largePayload {
		largePayload[i] = 'A'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) != len(largePayload) {
			t.Errorf("Expected body size %d, got %d", len(largePayload), len(body))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	provider := NewProvider("test-provider", server.URL, "schema1", nil)
	ctx := context.Background()

	resp, err := provider.PerformRequest(ctx, largePayload)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
