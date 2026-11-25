// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package thunderid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openndx/openndx-core/internal/pb/idp"
)

const roleID = "role-getdata"

// fakeThunderID records the management API calls the client makes.
type fakeThunderID struct {
	t            *testing.T
	tokenForm    map[string]string
	created      *applicationRequest
	assigned     *assignmentsRequest
	deleted      []string
	assignStatus int
	// cancelOnAssign, if set, is called when the role assignment arrives.
	cancelOnAssign context.CancelFunc
}

func (f *fakeThunderID) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/oauth2/token":
		require.NoError(f.t, r.ParseForm())
		f.tokenForm = map[string]string{"scope": r.PostForm.Get("scope"), "resource": r.PostForm.Get("resource")}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pb-token", "token_type": "Bearer", "expires_in": 3600})
	case r.Method == http.MethodPost && r.URL.Path == "/applications":
		assert.Equal(f.t, "Bearer pb-token", r.Header.Get("Authorization"))
		f.created = &applicationRequest{}
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(f.created))
		if f.created.Name == "NoSecret" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"app-1","clientId":"client-1","inboundAuthConfig":[]}`))
			return
		}
		if f.created.Name == "Dup" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"APP-1020","message":{"defaultValue":"Application already exists"},"description":{"defaultValue":"An application with the same name already exists"}}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"app-1","ouId":"` + f.created.OuID + `","name":"` + f.created.Name + `","clientId":"client-1",
			"inboundAuthConfig":[{"type":"oauth2","config":{"clientId":"client-1","clientSecret":"secret-1"}}]}`))
	case r.Method == http.MethodPost && r.URL.Path == "/roles/"+roleID+"/assignments/add":
		f.assigned = &assignmentsRequest{}
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(f.assigned))
		if f.cancelOnAssign != nil {
			// The caller gives up mid-request: wait for the client to drop it.
			f.cancelOnAssign()
			<-r.Context().Done()
			return
		}
		if f.assignStatus != 0 {
			w.WriteHeader(f.assignStatus)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && r.URL.Path == "/applications/app-1":
		f.deleted = append(f.deleted, "app-1")
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestClient(t *testing.T, roleID string) (*Client, *fakeThunderID) {
	fake := &fakeThunderID{t: t}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return NewClient(Config{
		BaseURL:            server.URL,
		ClientID:           "PORTAL_BACKEND",
		ClientSecret:       "secret",
		Scopes:             []string{"system"},
		Resource:           "https://localhost:8090/mcp",
		DataConsumerRoleID: roleID,
	}), fake
}

func TestClient_CreateApplication(t *testing.T) {
	t.Run("creates an M2M client in the OU and assigns the data-consumer role", func(t *testing.T) {
		client, fake := newTestClient(t, roleID)

		created, err := client.CreateApplication(context.Background(), &idp.Application{
			Name: "Passport Application", Description: "d", OuID: "ou-die",
		})

		require.NoError(t, err)
		assert.Equal(t, &idp.CreatedApplication{Id: "app-1", ClientId: "client-1", ClientSecret: "secret-1"}, created)

		assert.Equal(t, map[string]string{"scope": "system", "resource": "https://localhost:8090/mcp"}, fake.tokenForm)
		require.NotNil(t, fake.created)
		assert.Equal(t, "ou-die", fake.created.OuID)
		assert.Equal(t, "m2m", fake.created.Type)
		require.Len(t, fake.created.InboundAuthConfig, 1)
		assert.Equal(t, []string{"client_credentials"}, fake.created.InboundAuthConfig[0].Config.GrantTypes)
		assert.False(t, fake.created.InboundAuthConfig[0].Config.PublicClient)

		require.NotNil(t, fake.assigned)
		assert.Equal(t, []assignment{{Type: "app", ID: "app-1"}}, fake.assigned.Assignments)
		assert.Empty(t, fake.deleted)
	})

	t.Run("skips the role assignment when no role is configured", func(t *testing.T) {
		client, fake := newTestClient(t, "")

		_, err := client.CreateApplication(context.Background(), &idp.Application{Name: "App", OuID: "ou-die"})

		require.NoError(t, err)
		assert.Nil(t, fake.assigned)
	})

	t.Run("deletes the application when the role assignment fails", func(t *testing.T) {
		client, fake := newTestClient(t, roleID)
		fake.assignStatus = http.StatusInternalServerError

		created, err := client.CreateApplication(context.Background(), &idp.Application{Name: "App", OuID: "ou-die"})

		assert.Nil(t, created)
		assert.ErrorContains(t, err, "failed to assign data-consumer role")
		assert.Equal(t, []string{"app-1"}, fake.deleted)
	})

	t.Run("still deletes the application when the request is canceled", func(t *testing.T) {
		client, fake := newTestClient(t, roleID)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fake.cancelOnAssign = cancel

		created, err := client.CreateApplication(ctx, &idp.Application{Name: "App", OuID: "ou-die"})

		assert.Nil(t, created)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, []string{"app-1"}, fake.deleted)
	})

	t.Run("rejects and deletes an application created without a client secret", func(t *testing.T) {
		client, fake := newTestClient(t, roleID)

		created, err := client.CreateApplication(context.Background(), &idp.Application{Name: "NoSecret", OuID: "ou-die"})

		assert.Nil(t, created)
		assert.ErrorContains(t, err, "no client id or secret")
		assert.Equal(t, []string{"app-1"}, fake.deleted)
		assert.Nil(t, fake.assigned)
	})

	t.Run("returns ThunderID's error message", func(t *testing.T) {
		client, _ := newTestClient(t, roleID)

		_, err := client.CreateApplication(context.Background(), &idp.Application{Name: "Dup", OuID: "ou-die"})

		assert.EqualError(t, err, "thunderid POST /applications: Application already exists (APP-1020): An application with the same name already exists")
	})

	t.Run("requires an organization unit", func(t *testing.T) {
		client, fake := newTestClient(t, roleID)

		_, err := client.CreateApplication(context.Background(), &idp.Application{Name: "App"})

		assert.ErrorContains(t, err, "organization unit is required")
		assert.Nil(t, fake.created)
	})
}

func TestClient_DeleteApplication(t *testing.T) {
	client, fake := newTestClient(t, roleID)

	require.NoError(t, client.DeleteApplication(context.Background(), "app-1"))
	assert.Equal(t, []string{"app-1"}, fake.deleted)

	err := client.DeleteApplication(context.Background(), "missing")
	assert.ErrorContains(t, err, "404")
}
