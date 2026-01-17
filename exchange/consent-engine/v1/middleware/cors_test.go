package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultCORSConfig(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("CORS_ALLOWED_ORIGINS")

	config := DefaultCORSConfig("http://localhost:5173")

	assert.Contains(t, config.AllowedOrigins, "http://localhost:5173")
	assert.Contains(t, config.AllowedMethods, "GET")
	assert.Contains(t, config.AllowedMethods, "POST")
	assert.Contains(t, config.AllowedHeaders, "Authorization")
	assert.Contains(t, config.AllowedHeaders, "Content-Type")
	assert.True(t, config.AllowCredentials)
	assert.Equal(t, 86400, config.MaxAge)
}

func TestDefaultCORSConfig_WithEnvVar(t *testing.T) {
	// Test that DefaultCORSConfig correctly parses comma-separated origins
	// This simulates how the config package combines CORS_ALLOWED_ORIGINS with consent portal URL
	config := DefaultCORSConfig("https://example.com,https://test.com,http://localhost:5173")

	assert.Contains(t, config.AllowedOrigins, "http://localhost:5173")
	assert.Contains(t, config.AllowedOrigins, "https://example.com")
	assert.Contains(t, config.AllowedOrigins, "https://test.com")
}

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins:   []string{"https://example.com"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	middleware := CORSMiddleware(config)

	req := httptest.NewRequest("GET", "/api/v1/consents", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware(next).ServeHTTP(w, req)

	assert.True(t, nextCalled)
	assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, Authorization", w.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "3600", w.Header().Get("Access-Control-Max-Age"))
	assert.Contains(t, w.Header().Values("Vary"), "Origin")
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
	}

	middleware := CORSMiddleware(config)

	req := httptest.NewRequest("GET", "/api/v1/consents", nil)
	req.Header.Set("Origin", "https://malicious.com")
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware(next).ServeHTTP(w, req)

	assert.True(t, nextCalled)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_WildcardOrigin(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET"},
		AllowCredentials: false, // Must be false with wildcard
	}

	middleware := CORSMiddleware(config)

	req := httptest.NewRequest("GET", "/api/v1/consents", nil)
	req.Header.Set("Origin", "https://any-origin.com")
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware(next).ServeHTTP(w, req)

	assert.True(t, nextCalled)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type"},
	}

	middleware := CORSMiddleware(config)

	req := httptest.NewRequest("OPTIONS", "/api/v1/consents", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware(next).ServeHTTP(w, req)

	assert.False(t, nextCalled)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Values("Vary"), "Access-Control-Request-Method")
	assert.Contains(t, w.Header().Values("Vary"), "Access-Control-Request-Headers")
}

func TestCORSMiddleware_WildcardWithCredentials_Panic(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true, // This should cause a panic
	}

	assert.Panics(t, func() {
		CORSMiddleware(config)
	})
}

func TestNewCORSMiddleware(t *testing.T) {
	os.Unsetenv("CORS_ALLOWED_ORIGINS")

	middleware := NewCORSMiddleware("http://localhost:5173")
	assert.NotNil(t, middleware)

	req := httptest.NewRequest("GET", "/api/v1/consents", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware(next).ServeHTTP(w, req)

	assert.True(t, nextCalled)
	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
}
