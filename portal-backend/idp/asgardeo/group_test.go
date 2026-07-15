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

func TestClient_CreateGroup(t *testing.T) {
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
			// Handle SCIM group creation
			if r.URL.Path == "/scim2/Groups" && r.Method == "POST" {
				assert.Equal(t, "application/scim+json", r.Header.Get("Content-Type"))
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(CreateGroupResponseBody{
					ID:          "group-123",
					DisplayName: "DEFAULT/Test Group",
					Members: []struct {
						Value   string `json:"value"`
						Display string `json:"display"`
					}{
						{Value: "user-123", Display: "Test User"},
					},
					Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		group := &idp.Group{
			DisplayName: "Test Group",
			Members: []*idp.GroupMember{
				{Value: "user-123", Display: "Test User"},
			},
		}

		result, err := client.CreateGroup(context.Background(), group)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "group-123", result.Id)
		// Asgardeo returns DisplayName with "DEFAULT/" prefix
		assert.Equal(t, "DEFAULT/Test Group", result.DisplayName)
		assert.Equal(t, 1, len(result.Members))
	})

	t.Run("Success_WithoutMembers", func(t *testing.T) {
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
			// Handle SCIM group creation
			if r.URL.Path == "/scim2/Groups" && r.Method == "POST" {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(CreateGroupResponseBody{
					ID:          "group-123",
					DisplayName: "DEFAULT/Test Group",
					Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		group := &idp.Group{
			DisplayName: "Test Group",
		}

		result, err := client.CreateGroup(context.Background(), group)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "group-123", result.Id)
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
			// Return 400 for group creation
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		group := &idp.Group{
			DisplayName: "Test Group",
		}

		result, err := client.CreateGroup(context.Background(), group)

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
		group := &idp.Group{
			DisplayName: "Test Group",
		}

		result, err := client.CreateGroup(context.Background(), group)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestClient_GetGroup(t *testing.T) {
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
			// Handle SCIM group get
			if r.URL.Path == "/scim2/Groups/group-123" && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(GetGroupResponseBody{
					ID:          "group-123",
					DisplayName: "DEFAULT/Test Group",
					Members: []struct {
						Value   string `json:"value"`
						Display string `json:"display"`
					}{
						{Value: "user-123", Display: "Test User"},
					},
					Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		groupInfo, err := client.GetGroup(context.Background(), "group-123")

		assert.NoError(t, err)
		assert.NotNil(t, groupInfo)
		assert.Equal(t, "group-123", groupInfo.Id)
		assert.Equal(t, "Test Group", groupInfo.DisplayName)
		assert.Equal(t, 1, len(groupInfo.Members))
	})

	t.Run("Success_WithoutMembers", func(t *testing.T) {
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
			// Handle SCIM group get
			if r.URL.Path == "/scim2/Groups/group-123" && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(GetGroupResponseBody{
					ID:          "group-123",
					DisplayName: "DEFAULT/Test Group",
					Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		groupInfo, err := client.GetGroup(context.Background(), "group-123")

		assert.NoError(t, err)
		assert.NotNil(t, groupInfo)
		assert.Equal(t, "group-123", groupInfo.Id)
		assert.Equal(t, 0, len(groupInfo.Members))
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
			// Return 404 for group get
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		groupInfo, err := client.GetGroup(context.Background(), "group-123")

		assert.Error(t, err)
		assert.Nil(t, groupInfo)
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
		groupInfo, err := client.GetGroup(context.Background(), "group-123")

		assert.Error(t, err)
		assert.Nil(t, groupInfo)
	})
}

func TestClient_GetGroupByName(t *testing.T) {
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
			// Handle SCIM group search
			if r.URL.Path == "/scim2/Groups/.search" && r.Method == "POST" {
				assert.Equal(t, "application/scim+json", r.Header.Get("Content-Type"))
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"totalResults": 1,
					"Resources": []GetGroupResponseBody{
						{
							ID:          "group-123",
							DisplayName: "DEFAULT/Test Group",
							Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
						},
					},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		groupID, err := client.GetGroupByName(context.Background(), "Test Group")

		assert.NoError(t, err)
		assert.NotNil(t, groupID)
		assert.Equal(t, "group-123", *groupID)
	})

	t.Run("NotFound", func(t *testing.T) {
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
			// Handle SCIM group search - no results
			if r.URL.Path == "/scim2/Groups/.search" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"totalResults": 0,
					"Resources":    []GetGroupResponseBody{},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		groupID, err := client.GetGroupByName(context.Background(), "NonExistent Group")

		assert.Error(t, err)
		assert.Nil(t, groupID)
		assert.Contains(t, err.Error(), "not found")
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
			// Return 400 for search
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		groupID, err := client.GetGroupByName(context.Background(), "Test Group")

		assert.Error(t, err)
		assert.Nil(t, groupID)
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
		groupID, err := client.GetGroupByName(context.Background(), "Test Group")

		assert.Error(t, err)
		assert.Nil(t, groupID)
	})
}

func TestClient_UpdateGroup(t *testing.T) {
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
			// Handle SCIM group update
			if r.URL.Path == "/scim2/Groups/group-123" && r.Method == "PUT" {
				assert.Equal(t, "application/scim+json", r.Header.Get("Content-Type"))
				json.NewEncoder(w).Encode(CreateGroupResponseBody{
					ID:          "group-123",
					DisplayName: "DEFAULT/Updated Group",
					Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		group := &idp.Group{
			DisplayName: "Updated Group",
		}

		result, err := client.UpdateGroup(context.Background(), "group-123", group)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "group-123", result.Id)
		assert.Equal(t, "Updated Group", result.DisplayName)
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
			// Return 404 for group update
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		group := &idp.Group{
			DisplayName: "Updated Group",
		}

		result, err := client.UpdateGroup(context.Background(), "group-123", group)

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
		group := &idp.Group{
			DisplayName: "Updated Group",
		}

		result, err := client.UpdateGroup(context.Background(), "group-123", group)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestClient_DeleteGroup(t *testing.T) {
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
			// Handle SCIM group deletion
			if r.URL.Path == "/scim2/Groups/group-123" && r.Method == "DELETE" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.DeleteGroup(context.Background(), "group-123")

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
			// Return 404 for group deletion
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.DeleteGroup(context.Background(), "group-123")

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
		err := client.DeleteGroup(context.Background(), "group-123")

		assert.Error(t, err)
	})
}

func TestClient_AddMemberToGroup(t *testing.T) {
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
			// Handle SCIM group patch (add member)
			if r.URL.Path == "/scim2/Groups/group-123" && r.Method == "PATCH" {
				assert.Equal(t, "application/scim+json", r.Header.Get("Content-Type"))
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(CreateGroupResponseBody{
					ID:          "group-123",
					DisplayName: "DEFAULT/Test Group",
					Members: []struct {
						Value   string `json:"value"`
						Display string `json:"display"`
					}{
						{Value: "user-123", Display: "Test User"},
					},
					Schemas: []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		groupID := "group-123"
		member := &idp.GroupMember{
			Value:   "user-123",
			Display: "Test User",
		}

		err := client.AddMemberToGroup(context.Background(), &groupID, member)

		assert.NoError(t, err)
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
			// Return 404 for patch
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		groupID := "group-123"
		member := &idp.GroupMember{
			Value:   "user-123",
			Display: "Test User",
		}

		err := client.AddMemberToGroup(context.Background(), &groupID, member)

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
		groupID := "group-123"
		member := &idp.GroupMember{
			Value:   "user-123",
			Display: "Test User",
		}

		err := client.AddMemberToGroup(context.Background(), &groupID, member)

		assert.Error(t, err)
	})
}

func TestClient_AddMemberToGroupByGroupName(t *testing.T) {
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
			// Handle SCIM group search
			if r.URL.Path == "/scim2/Groups/.search" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"totalResults": 1,
					"Resources": []GetGroupResponseBody{
						{
							ID:          "group-123",
							DisplayName: "DEFAULT/Test Group",
							Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
						},
					},
				})
				return
			}
			// Handle SCIM group patch (add member)
			if r.URL.Path == "/scim2/Groups/group-123" && r.Method == "PATCH" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(CreateGroupResponseBody{
					ID:          "group-123",
					DisplayName: "DEFAULT/Test Group",
					Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		member := &idp.GroupMember{
			Value:   "user-123",
			Display: "Test User",
		}

		groupID, err := client.AddMemberToGroupByGroupName(context.Background(), "Test Group", member)

		assert.NoError(t, err)
		assert.NotNil(t, groupID)
		assert.Equal(t, "group-123", *groupID)
	})

	t.Run("GroupNotFound", func(t *testing.T) {
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
			// Handle SCIM group search - no results
			if r.URL.Path == "/scim2/Groups/.search" && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"totalResults": 0,
					"Resources":    []GetGroupResponseBody{},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		member := &idp.GroupMember{
			Value:   "user-123",
			Display: "Test User",
		}

		groupID, err := client.AddMemberToGroupByGroupName(context.Background(), "NonExistent Group", member)

		assert.Error(t, err)
		assert.Nil(t, groupID)
		assert.Contains(t, err.Error(), "not found")
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
		member := &idp.GroupMember{
			Value:   "user-123",
			Display: "Test User",
		}

		groupID, err := client.AddMemberToGroupByGroupName(context.Background(), "Test Group", member)

		assert.Error(t, err)
		assert.Nil(t, groupID)
	})
}

func TestClient_RemoveMemberFromGroup(t *testing.T) {
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
			// Handle SCIM group patch (remove member)
			if r.URL.Path == "/scim2/Groups/group-123" && r.Method == "PATCH" {
				assert.Equal(t, "application/scim+json", r.Header.Get("Content-Type"))
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(CreateGroupResponseBody{
					ID:          "group-123",
					DisplayName: "DEFAULT/Test Group",
					Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.RemoveMemberFromGroup(context.Background(), "group-123", "user-123")

		assert.NoError(t, err)
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
			// Return 404 for patch
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient(server.URL, "client-id", "client-secret", []string{})
		err := client.RemoveMemberFromGroup(context.Background(), "group-123", "user-123")

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
		err := client.RemoveMemberFromGroup(context.Background(), "group-123", "user-123")

		assert.Error(t, err)
	})
}
