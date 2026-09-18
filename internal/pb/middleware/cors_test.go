package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultCORSConfig(t *testing.T) {
	t.Run("DefaultCORSConfig_WithEnvVar", func(t *testing.T) {
		os.Setenv("CORS_ALLOWED_ORIGINS", "http://example.com,http://test.com")
		defer os.Unsetenv("CORS_ALLOWED_ORIGINS")

		config := DefaultCORSConfig()

		assert.Equal(t, 2, len(config.AllowedOrigins))
		assert.Contains(t, config.AllowedOrigins, "http://example.com")
		assert.Contains(t, config.AllowedOrigins, "http://test.com")
		assert.True(t, config.AllowCredentials)
		assert.Equal(t, 86400, config.MaxAge)
	})

	t.Run("DefaultCORSConfig_WithoutEnvVar", func(t *testing.T) {
		os.Unsetenv("CORS_ALLOWED_ORIGINS")

		config := DefaultCORSConfig()

		assert.Equal(t, 1, len(config.AllowedOrigins))
		assert.Contains(t, config.AllowedOrigins, "http://localhost:5173")
	})
}

func TestCORSMiddleware(t *testing.T) {
	t.Run("CORSMiddleware_AllowedOrigin", func(t *testing.T) {
		config := CORSConfig{
			AllowedOrigins:   []string{"http://example.com"},
			AllowedMethods:   []string{"GET", "POST"},
			AllowedHeaders:   []string{"Content-Type"},
			AllowCredentials: true,
			MaxAge:           3600,
		}

		middleware := CORSMiddleware(config)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, "http://example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
		assert.Equal(t, "3600", w.Header().Get("Access-Control-Max-Age"))
	})

	t.Run("CORSMiddleware_DisallowedOrigin", func(t *testing.T) {
		config := CORSConfig{
			AllowedOrigins: []string{"http://example.com"},
		}

		middleware := CORSMiddleware(config)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://malicious.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("CORSMiddleware_WildcardOrigin", func(t *testing.T) {
		config := CORSConfig{
			AllowedOrigins: []string{"*"},
		}

		middleware := CORSMiddleware(config)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://any.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("CORSMiddleware_OPTIONS", func(t *testing.T) {
		config := CORSConfig{
			AllowedOrigins: []string{"http://example.com"},
		}

		middleware := CORSMiddleware(config)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("CORSMiddleware_ExposedHeaders", func(t *testing.T) {
		config := CORSConfig{
			AllowedOrigins: []string{"http://example.com"},
			ExposedHeaders: []string{"X-Custom-Header"},
		}

		middleware := CORSMiddleware(config)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, "X-Custom-Header", w.Header().Get("Access-Control-Expose-Headers"))
	})
}

func TestNewCORSMiddleware(t *testing.T) {
	middleware := NewCORSMiddleware()
	assert.NotNil(t, middleware)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
