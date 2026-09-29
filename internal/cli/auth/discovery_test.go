package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiscoverEndpoints_Success(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_endpoint":"https://idp.example.com/oauth2/authorize","token_endpoint":"https://idp.example.com/oauth2/token"}`))
	}))
	defer server.Close()

	authURL, tokenURL, err := DiscoverEndpoints(context.Background(), server.Client(), server.URL)
	assert.NoError(t, err)
	assert.Equal(t, "https://idp.example.com/oauth2/authorize", authURL)
	assert.Equal(t, "https://idp.example.com/oauth2/token", tokenURL)
	assert.Equal(t, "/.well-known/openid-configuration", requestedPath)
}

func TestDiscoverEndpoints_TrailingSlashOnIssuer(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_endpoint":"https://idp.example.com/oauth2/authorize","token_endpoint":"https://idp.example.com/oauth2/token"}`))
	}))
	defer server.Close()

	_, _, err := DiscoverEndpoints(context.Background(), server.Client(), server.URL+"/")
	assert.NoError(t, err)
	assert.Equal(t, "/.well-known/openid-configuration", requestedPath)
}

func TestDiscoverEndpoints_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	_, _, err := DiscoverEndpoints(context.Background(), server.Client(), server.URL)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "status 404")
}

func TestDiscoverEndpoints_ResponseTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		oversized := make([]byte, maxDiscoveryResponseBytes+1)
		for i := range oversized {
			oversized[i] = ' '
		}
		_, _ = w.Write(oversized)
	}))
	defer server.Close()

	_, _, err := DiscoverEndpoints(context.Background(), server.Client(), server.URL)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "larger than")
}

func TestDiscoverEndpoints_MissingEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"https://idp.example.com"}`))
	}))
	defer server.Close()

	_, _, err := DiscoverEndpoints(context.Background(), server.Client(), server.URL)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "missing authorization_endpoint or token_endpoint")
}
