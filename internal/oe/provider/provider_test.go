// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package provider

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpenNSW/core/remote"
	remoteauth "github.com/OpenNSW/core/remote/auth"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry_AndGet(t *testing.T) {
	registry, err := NewRegistry(
		Config{ID: "drp-v1", URL: "http://example1.com"},
		Config{ID: "drp-v2", URL: "http://example1.com", Timeout: "5s"},
		Config{ID: "rgd-v1", URL: "http://example3.com/graphql"},
	)
	require.NoError(t, err)

	p1, ok := registry.Get("drp-v1")
	require.True(t, ok)
	assert.Equal(t, "http://example1.com", p1.url)

	p2, ok := registry.Get("drp-v2")
	require.True(t, ok, "two schemas may share an endpoint")
	assert.Equal(t, "http://example1.com", p2.url)

	p3, ok := registry.Get("rgd-v1")
	require.True(t, ok)
	assert.Equal(t, "http://example3.com/graphql", p3.url)

	_, ok = registry.Get("unknown")
	assert.False(t, ok)

	var nilRegistry *Registry
	_, ok = nilRegistry.Get("drp-v1")
	assert.False(t, ok)
}

func TestNewRegistry_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		configs []Config
		wantErr string
	}{
		{
			name:    "missing id",
			configs: []Config{{URL: "http://example.com"}},
			wantErr: "providers[0].id is required",
		},
		{
			name:    "missing url",
			configs: []Config{{ID: "drp-v1"}},
			wantErr: "providers[0].url",
		},
		{
			name:    "non-http url",
			configs: []Config{{ID: "drp-v1", URL: "ftp://example.com"}},
			wantErr: "providers[0].url",
		},
		{
			name: "duplicate id",
			configs: []Config{
				{ID: "drp-v1", URL: "http://example1.com"},
				{ID: "drp-v1", URL: "http://example2.com"},
			},
			wantErr: `duplicate provider for schema "drp-v1"`,
		},
		{
			name:    "invalid timeout",
			configs: []Config{{ID: "drp-v1", URL: "http://example.com", Timeout: "not-a-duration"}},
			wantErr: `providers[0] (drp-v1): invalid timeout "not-a-duration"`,
		},
		{
			name: "invalid auth config",
			configs: []Config{{
				ID:   "drp-v1",
				URL:  "http://example.com",
				Auth: &remoteauth.Config{Type: remoteauth.TypeOAuth2},
			}},
			wantErr: `missing "oauth2" settings`,
		},
		{
			name: "incomplete tls config",
			configs: []Config{{
				ID:  "drp-v1",
				URL: "http://example.com",
				TLS: &remote.TLSSettings{ClientCertFile: "/etc/cert.pem"},
			}},
			wantErr: "tls config requires both clientCertFile and clientKeyFile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRegistry(tt.configs...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNewRegistry_WarnsAboutCredentialsOverPlainHTTP(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	_, err := NewRegistry(
		Config{ID: "open-v1", URL: "http://open.example"},
		Config{
			ID:   "drp-v1",
			URL:  "https://drp.example",
			Auth: &remoteauth.Config{Type: remoteauth.TypeAPIKey, APIKey: &remoteauth.APIKeyConfig{Key: "X-API-Key", Value: "k"}},
		},
		Config{
			ID:  "rgd-v1",
			URL: "http://rgd.example/graphql",
			Auth: &remoteauth.Config{
				Type:   remoteauth.TypeOAuth2,
				OAuth2: &remoteauth.OAuth2Config{TokenURL: "http://rgd.example/oauth/token", ClientID: "c", ClientSecret: "s"},
			},
		},
	)
	require.NoError(t, err, "plain HTTP stays allowed")

	out := logs.String()
	assert.NotContains(t, out, "open-v1", "a provider without credentials isn't warned about")
	assert.NotContains(t, out, "drp-v1", "credentials over https aren't warned about")
	assert.Contains(t, out, "key=providers[2].url")
	assert.Contains(t, out, "key=providers[2].auth.oauth2.tokenURL")
}

func TestProvider_Execute(t *testing.T) {
	t.Run("sends the GraphQL request to the exact configured URL and decodes the response", func(t *testing.T) {
		gotPath := make(chan string, 1)
		gotContentType := make(chan string, 1)
		gotBody := make(chan string, 1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			gotPath <- r.URL.Path
			gotContentType <- r.Header.Get("Content-Type")
			gotBody <- string(body)
			_, _ = w.Write([]byte(`{"data":{"person":{"name":"Jane"}}}`))
		}))
		t.Cleanup(server.Close)

		registry, err := NewRegistry(Config{ID: "drp-v1", URL: server.URL + "/graphql"})
		require.NoError(t, err)
		p, ok := registry.Get("drp-v1")
		require.True(t, ok)

		resp, err := p.Execute(context.Background(), graphql.Request{Query: "query { person { name } }"})
		require.NoError(t, err)
		assert.Equal(t, "/graphql", <-gotPath)
		assert.Equal(t, "application/json", <-gotContentType)
		assert.Equal(t, `{"query":"query { person { name } }"}`, <-gotBody)
		person, _ := resp.Data["person"].(map[string]any)
		assert.Equal(t, "Jane", person["name"])
	})

	t.Run("applies API key authentication", func(t *testing.T) {
		gotKey := make(chan string, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey <- r.Header.Get("X-API-Key")
			_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
		}))
		t.Cleanup(server.Close)

		registry, err := NewRegistry(Config{
			ID:  "drp-v1",
			URL: server.URL,
			Auth: &remoteauth.Config{
				Type:   remoteauth.TypeAPIKey,
				APIKey: &remoteauth.APIKeyConfig{Key: "X-API-Key", Value: "secret-key-123"},
			},
		})
		require.NoError(t, err)
		p, _ := registry.Get("drp-v1")

		_, err = p.Execute(context.Background(), graphql.Request{Query: "{ ok }"})
		require.NoError(t, err)
		assert.Equal(t, "secret-key-123", <-gotKey)
	})

	t.Run("applies OAuth2 client-credentials authentication", func(t *testing.T) {
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"oauth-token-xyz","token_type":"Bearer","expires_in":3600}`))
		}))
		t.Cleanup(tokenServer.Close)

		gotAuth := make(chan string, 1)
		resourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth <- r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
		}))
		t.Cleanup(resourceServer.Close)

		registry, err := NewRegistry(Config{
			ID:  "rgd-v1",
			URL: resourceServer.URL + "/graphql",
			Auth: &remoteauth.Config{
				Type: remoteauth.TypeOAuth2,
				OAuth2: &remoteauth.OAuth2Config{
					TokenURL:     tokenServer.URL,
					ClientID:     "client-id",
					ClientSecret: "client-secret",
					Scopes:       []string{"read"},
				},
			},
		})
		require.NoError(t, err)
		p, _ := registry.Get("rgd-v1")

		_, err = p.Execute(context.Background(), graphql.Request{Query: "{ ok }"})
		require.NoError(t, err)
		assert.Equal(t, "Bearer oauth-token-xyz", <-gotAuth)
	})

	t.Run("returns an error when the response is not JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("provider unavailable"))
		}))
		t.Cleanup(server.Close)

		registry, err := NewRegistry(Config{ID: "drp-v1", URL: server.URL})
		require.NoError(t, err)
		p, _ := registry.Get("drp-v1")

		_, err = p.Execute(context.Background(), graphql.Request{Query: "query { a }"})
		require.Error(t, err)
	})

	t.Run("returns an error for a non-2xx status even with a JSON body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"data":{"a":"stale"}}`))
		}))
		t.Cleanup(server.Close)

		registry, err := NewRegistry(Config{ID: "drp-v1", URL: server.URL})
		require.NoError(t, err)
		p, _ := registry.Get("drp-v1")

		_, err = p.Execute(context.Background(), graphql.Request{Query: "query { a }"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider for schema drp-v1 returned HTTP status 503")
	})

	t.Run("returns an error when the context is cancelled", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		registry, err := NewRegistry(Config{ID: "drp-v1", URL: server.URL})
		require.NoError(t, err)
		p, _ := registry.Get("drp-v1")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = p.Execute(ctx, graphql.Request{Query: "query { a }"})
		require.Error(t, err)
	})

	t.Run("returns an error when the provider is unreachable", func(t *testing.T) {
		registry, err := NewRegistry(Config{ID: "drp-v1", URL: "http://127.0.0.1:0"})
		require.NoError(t, err)
		p, _ := registry.Get("drp-v1")

		_, err = p.Execute(context.Background(), graphql.Request{Query: "query { a }"})
		require.Error(t, err)
	})
}
