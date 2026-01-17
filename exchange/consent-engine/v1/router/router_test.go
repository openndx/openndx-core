package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/auth"
	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/handlers"
	"github.com/stretchr/testify/assert"
)

func TestNewV1Router(t *testing.T) {
	internalHandler := &handlers.InternalHandler{}
	portalHandler := &handlers.PortalHandler{}

	// Create a minimal JWT verifier config for testing
	config := auth.JWTVerifierConfig{
		JWKSUrl:  "http://localhost/.well-known/jwks.json",
		Issuer:   "test-issuer",
		Audience: "test-audience",
	}
	jwtVerifier, err := auth.NewJWTVerifier(config)
	if err != nil {
		// Skip test if JWT verifier creation fails (e.g., network issue)
		t.Skipf("Failed to create JWT verifier: %v", err)
	}

	router := NewV1Router("http://localhost:5173", internalHandler, portalHandler, jwtVerifier)

	assert.NotNil(t, router)
	assert.Equal(t, internalHandler, router.internalHandler)
	assert.Equal(t, portalHandler, router.portalHandler)
	assert.NotNil(t, router.authMiddleware)
	assert.NotNil(t, router.corsMiddleware)
}

func TestV1Router_RegisterRoutes(t *testing.T) {
	internalHandler := &handlers.InternalHandler{}
	portalHandler := &handlers.PortalHandler{}

	config := auth.JWTVerifierConfig{
		JWKSUrl:  "http://localhost/.well-known/jwks.json",
		Issuer:   "test-issuer",
		Audience: "test-audience",
	}
	jwtVerifier, err := auth.NewJWTVerifier(config)
	if err != nil {
		t.Skipf("Failed to create JWT verifier: %v", err)
	}

	router := NewV1Router("http://localhost:5173", internalHandler, portalHandler, jwtVerifier)
	mux := http.NewServeMux()

	router.RegisterRoutes(mux)

	// Test that internal routes are registered
	req := httptest.NewRequest("GET", "/internal/api/v1/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should not return 404 (route exists)
	assert.NotEqual(t, http.StatusNotFound, w.Code)

	// Test that portal routes are registered
	req = httptest.NewRequest("GET", "/api/v1/health", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should not return 404 (route exists)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestV1Router_ApplyCORS(t *testing.T) {
	internalHandler := &handlers.InternalHandler{}
	portalHandler := &handlers.PortalHandler{}

	config := auth.JWTVerifierConfig{
		JWKSUrl:  "http://localhost/.well-known/jwks.json",
		Issuer:   "test-issuer",
		Audience: "test-audience",
	}
	jwtVerifier, err := auth.NewJWTVerifier(config)
	if err != nil {
		t.Skipf("Failed to create JWT verifier: %v", err)
	}

	router := NewV1Router("http://localhost:5173", internalHandler, portalHandler, jwtVerifier)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := router.ApplyCORS(handler)

	assert.NotNil(t, wrapped)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	// CORS headers should be set
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

