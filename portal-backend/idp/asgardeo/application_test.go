package asgardeo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenNDX/openndx-core/portal-backend/idp"
	"github.com/stretchr/testify/assert"
)

func TestClient_GetApplicationInfo(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Handle application info request
			if strings.Contains(r.URL.Path, "/api/server/v1/applications/") && r.Method == "GET" && !strings.Contains(r.URL.Path, "/inbound-protocols/") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(AsgardeoApplicationInfo{
					Id:          "app-123",
					Name:        "Test Application",
					Description: "Test Description",
					ClientId:    "client-123",
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		appInfo, err := client.GetApplicationInfo(context.Background(), "app-123")

		assert.NoError(t, err)
		assert.NotNil(t, appInfo)
		assert.Equal(t, "app-123", appInfo.Id)
		assert.Equal(t, "Test Application", appInfo.Name)
		assert.Equal(t, "Test Description", appInfo.Description)
		assert.Equal(t, "client-123", appInfo.ClientId)
	})

	t.Run("HTTPError", func(t *testing.T) {
		// Use a server that will fail after OAuth token is obtained
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
		}))
		defer server.Close()
		server.Close() // Close immediately to cause connection error

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		appInfo, err := client.GetApplicationInfo(context.Background(), "app-123")

		assert.Error(t, err)
		assert.Nil(t, appInfo)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Return invalid JSON
			if strings.Contains(r.URL.Path, "/api/server/v1/applications/") && r.Method == "GET" && !strings.Contains(r.URL.Path, "/inbound-protocols/") {
				w.Write([]byte("invalid json"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		appInfo, err := client.GetApplicationInfo(context.Background(), "app-123")

		assert.Error(t, err)
		assert.Nil(t, appInfo)
	})
}

func TestClient_GetApplicationOIDC(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Handle OIDC info request
			if strings.Contains(r.URL.Path, "/api/server/v1/applications/") && strings.Contains(r.URL.Path, "/inbound-protocols/oidc") && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(AsgardeoApplicationOIDCResponse{
					ClientId:     "client-123",
					ClientSecret: "secret-456",
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		oidcInfo, err := client.GetApplicationOIDC(context.Background(), "app-123")

		assert.NoError(t, err)
		assert.NotNil(t, oidcInfo)
		assert.Equal(t, "client-123", oidcInfo.ClientId)
		assert.Equal(t, "secret-456", oidcInfo.ClientSecret)
	})

	t.Run("Non200Status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Return 404 for OIDC request
			if strings.Contains(r.URL.Path, "/inbound-protocols/oidc") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		oidcInfo, err := client.GetApplicationOIDC(context.Background(), "app-123")

		assert.Error(t, err)
		assert.Nil(t, oidcInfo)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("HTTPError", func(t *testing.T) {
		// Use a server that will fail after OAuth token is obtained
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
		}))
		defer server.Close()
		server.Close() // Close immediately to cause connection error

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		oidcInfo, err := client.GetApplicationOIDC(context.Background(), "app-123")

		assert.Error(t, err)
		assert.Nil(t, oidcInfo)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Return invalid JSON
			if strings.Contains(r.URL.Path, "/inbound-protocols/oidc") {
				w.Write([]byte("invalid json"))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		oidcInfo, err := client.GetApplicationOIDC(context.Background(), "app-123")

		assert.Error(t, err)
		assert.Nil(t, oidcInfo)
	})
}

func TestClient_CreateApplication(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		var serverURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Handle application creation
			if r.URL.Path == "/api/server/v1/applications/" && r.Method == "POST" {
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				// Location header should be full URL (as Asgardeo returns it)
				w.Header().Set("Location", serverURL+"/api/server/v1/applications/app-123")
				w.WriteHeader(http.StatusCreated)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		serverURL = server.URL

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		app := &idp.Application{
			Name:        "Test Application",
			Description: "Test Description",
			TemplateId:  "m2m-application",
		}

		appID, err := client.CreateApplication(context.Background(), app)

		assert.NoError(t, err)
		assert.NotNil(t, appID)
		assert.Equal(t, "app-123", *appID)
	})

	t.Run("Non200Status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Return 400 for application creation
			if r.URL.Path == "/api/server/v1/applications/" && r.Method == "POST" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		app := &idp.Application{
			Name:        "Test Application",
			Description: "Test Description",
			TemplateId:  "m2m-application",
		}

		appID, err := client.CreateApplication(context.Background(), app)

		assert.Error(t, err)
		assert.Nil(t, appID)
		assert.Contains(t, err.Error(), "status")
	})

	t.Run("HTTPError", func(t *testing.T) {
		// Use a server that will fail after OAuth token is obtained
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
		}))
		defer server.Close()
		server.Close() // Close immediately to cause connection error

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		app := &idp.Application{
			Name:        "Test Application",
			Description: "Test Description",
			TemplateId:  "m2m-application",
		}

		appID, err := client.CreateApplication(context.Background(), app)

		assert.Error(t, err)
		assert.Nil(t, appID)
	})

	t.Run("NoLocationHeader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Return success but no Location header
			if r.URL.Path == "/api/server/v1/applications/" && r.Method == "POST" {
				w.WriteHeader(http.StatusCreated)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		app := &idp.Application{
			Name:        "Test Application",
			Description: "Test Description",
			TemplateId:  "m2m-application",
		}

		appID, err := client.CreateApplication(context.Background(), app)

		// Should return empty string, not error
		assert.NoError(t, err)
		// The implementation returns empty string if Location header is missing
		if appID != nil {
			assert.Equal(t, "", *appID)
		}
	})
}

func TestClient_DeleteApplication(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Handle application deletion
			if strings.Contains(r.URL.Path, "/api/server/v1/applications/") && r.Method == "DELETE" && !strings.Contains(r.URL.Path, "/inbound-protocols/") {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.DeleteApplication(context.Background(), "app-123")

		assert.NoError(t, err)
	})

	t.Run("Non204Status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle OAuth token request
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
			// Return 404 for application deletion
			if strings.Contains(r.URL.Path, "/api/server/v1/applications/") && r.Method == "DELETE" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.DeleteApplication(context.Background(), "app-123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "status")
	})

	t.Run("HTTPError", func(t *testing.T) {
		// Use a server that will fail after OAuth token is obtained
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/oauth2/token" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}
		}))
		defer server.Close()
		server.Close() // Close immediately to cause connection error

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.DeleteApplication(context.Background(), "app-123")

		assert.Error(t, err)
	})
}
