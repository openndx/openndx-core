package asgardeo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNDX/openndx-core/portal-backend/idp"
	"github.com/stretchr/testify/assert"
)

func TestClient_GetUser(t *testing.T) {
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
			// Handle SCIM user request
			if r.URL.Path == "/scim2/Users/user-123" && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(GetUserResponseBody{
					ID:       "user-123",
					UserName: "DEFAULT/test@example.com",
					Email:    []string{"test@example.com"},
					PhoneNumbers: []struct {
						Value string `json:"value"`
						Type  string `json:"type"`
					}{
						{Value: "1234567890", Type: "mobile"},
					},
					Name: struct {
						FamilyName string `json:"familyName"`
						GivenName  string `json:"givenName"`
					}{
						GivenName:  "John",
						FamilyName: "Doe",
					},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo, err := client.GetUser(context.Background(), "user-123")

		assert.NoError(t, err)
		assert.NotNil(t, userInfo)
		assert.Equal(t, "user-123", userInfo.Id)
		assert.Equal(t, "John", userInfo.FirstName)
		assert.Equal(t, "Doe", userInfo.LastName)
		assert.Equal(t, "test@example.com", userInfo.Email)
		assert.Equal(t, "1234567890", userInfo.PhoneNumber)
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
			// Return 404 for user request
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo, err := client.GetUser(context.Background(), "user-123")

		assert.Error(t, err)
		assert.Nil(t, userInfo)
		assert.Contains(t, err.Error(), "status code: 404")
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
			// Close connection to simulate error
			w.Header().Set("Connection", "close")
		}))
		defer server.Close()
		server.Close() // Close immediately to cause connection error

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo, err := client.GetUser(context.Background(), "user-123")

		assert.Error(t, err)
		assert.Nil(t, userInfo)
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
			// Return invalid JSON for user request
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo, err := client.GetUser(context.Background(), "user-123")

		assert.Error(t, err)
		assert.Nil(t, userInfo)
	})
}

func TestClient_CreateUser(t *testing.T) {
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
			// Handle SCIM user creation
			if r.URL.Path == "/scim2/Users" && r.Method == "POST" {
				assert.Equal(t, "application/scim+json", r.Header.Get("Content-Type"))
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(CreateUserResponseBody{
					ID:       "user-123",
					UserName: "DEFAULT/test@example.com",
					Name: struct {
						FamilyName string `json:"familyName"`
						GivenName  string `json:"givenName"`
					}{
						GivenName:  "John",
						FamilyName: "Doe",
					},
					Emails: []string{"test@example.com"},
					PhoneNumbers: []struct {
						Value string `json:"value"`
						Type  string `json:"type"`
					}{
						{Value: "1234567890", Type: "mobile"},
					},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo := &idp.User{
			FirstName:   "John",
			LastName:    "Doe",
			Email:       "test@example.com",
			PhoneNumber: "1234567890",
		}

		result, err := client.CreateUser(context.Background(), userInfo)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "user-123", result.Id)
		assert.Equal(t, "John", result.FirstName)
		assert.Equal(t, "Doe", result.LastName)
		assert.Equal(t, "test@example.com", result.Email)
		assert.Equal(t, "1234567890", result.PhoneNumber)
	})

	t.Run("Success_WithoutPhone", func(t *testing.T) {
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
			// Handle SCIM user creation
			if r.URL.Path == "/scim2/Users" && r.Method == "POST" {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(CreateUserResponseBody{
					ID:       "user-123",
					UserName: "DEFAULT/test@example.com",
					Name: struct {
						FamilyName string `json:"familyName"`
						GivenName  string `json:"givenName"`
					}{
						GivenName:  "John",
						FamilyName: "Doe",
					},
					Emails: []string{"test@example.com"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo := &idp.User{
			FirstName: "John",
			LastName:  "Doe",
			Email:     "test@example.com",
		}

		result, err := client.CreateUser(context.Background(), userInfo)

		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("Non201Status", func(t *testing.T) {
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
			// Return 400 for user creation
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo := &idp.User{
			FirstName: "John",
			LastName:  "Doe",
			Email:     "test@example.com",
		}

		result, err := client.CreateUser(context.Background(), userInfo)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "status code: 400")
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
		userInfo := &idp.User{
			FirstName: "John",
			LastName:  "Doe",
			Email:     "test@example.com",
		}

		result, err := client.CreateUser(context.Background(), userInfo)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestClient_UpdateUser(t *testing.T) {
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
			// Handle SCIM user update
			if r.URL.Path == "/scim2/Users/user-123" && r.Method == "PUT" {
				assert.Equal(t, "application/scim+json", r.Header.Get("Content-Type"))
				json.NewEncoder(w).Encode(CreateUserResponseBody{
					ID:       "user-123",
					UserName: "DEFAULT/test@example.com",
					Name: struct {
						FamilyName string `json:"familyName"`
						GivenName  string `json:"givenName"`
					}{
						GivenName:  "Jane",
						FamilyName: "Smith",
					},
					Emails: []string{"jane@example.com"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo := &idp.User{
			FirstName: "Jane",
			LastName:  "Smith",
			Email:     "jane@example.com",
		}

		result, err := client.UpdateUser(context.Background(), "user-123", userInfo)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "user-123", result.Id)
		assert.Equal(t, "Jane", result.FirstName)
		assert.Equal(t, "Smith", result.LastName)
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
			// Return 404 for user update
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		userInfo := &idp.User{
			FirstName: "Jane",
			LastName:  "Smith",
			Email:     "jane@example.com",
		}

		result, err := client.UpdateUser(context.Background(), "user-123", userInfo)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "status code: 404")
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
		userInfo := &idp.User{
			FirstName: "Jane",
			LastName:  "Smith",
			Email:     "jane@example.com",
		}

		result, err := client.UpdateUser(context.Background(), "user-123", userInfo)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestClient_DeleteUser(t *testing.T) {
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
			// Handle SCIM user deletion
			if r.URL.Path == "/scim2/Users/user-123" && r.Method == "DELETE" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.DeleteUser(context.Background(), "user-123")

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
			// Return 404 for user deletion
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.DeleteUser(context.Background(), "user-123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "status code: 404")
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
		err := client.DeleteUser(context.Background(), "user-123")

		assert.Error(t, err)
	})
}
