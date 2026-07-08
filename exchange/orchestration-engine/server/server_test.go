package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
	"github.com/stretchr/testify/assert"
)

func init() {
	logger.Init()
}

func TestHealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp := Response{Message: "OpenDIF Server is Healthy!"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var response Response
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "OpenDIF Server is Healthy!", response.Message)
}

func TestHealthEndpoint_WrongMethod(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp := Response{Message: "OpenDIF Server is Healthy!"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestCorsMiddleware(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Check CORS headers
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST, PUT, DELETE, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCorsMiddleware_OptionsRequest(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// OPTIONS request should return 200 immediately
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestGetEnv(t *testing.T) {
	t.Run("Env var exists", func(t *testing.T) {
		os.Setenv("TEST_EXISTING_VAR", "test_value")
		defer os.Unsetenv("TEST_EXISTING_VAR")
		value := getEnv("TEST_EXISTING_VAR", "default")
		assert.Equal(t, "test_value", value)
	})

	t.Run("Env var does not exist", func(t *testing.T) {
		os.Unsetenv("TEST_NON_EXISTENT_VAR")
		value := getEnv("TEST_NON_EXISTENT_VAR", "default_value")
		assert.Equal(t, "default_value", value)
	})

	t.Run("Env var is empty string", func(t *testing.T) {
		os.Setenv("TEST_EMPTY_VAR", "")
		defer os.Unsetenv("TEST_EMPTY_VAR")
		value := getEnv("TEST_EMPTY_VAR", "default_value")
		assert.Equal(t, "default_value", value)
	})
}

func TestGetDatabaseConnectionString(t *testing.T) {
	t.Run("Standard environment variables set", func(t *testing.T) {
		t.Setenv("DB_HOST", "standard-host")
		t.Setenv("DB_PORT", "5434")
		t.Setenv("DB_USERNAME", "standard-user")
		t.Setenv("DB_PASSWORD", "standard-password")
		t.Setenv("DB_NAME", "standard-db")
		t.Setenv("DB_SSLMODE", "prefer")

		connStr := getDatabaseConnectionString()
		expected := "host=standard-host port=5434 user=standard-user password=standard-password dbname=standard-db sslmode=prefer"
		assert.Equal(t, expected, connStr)
	})

	t.Run("Standard environment variables with missing password", func(t *testing.T) {
		t.Setenv("DB_HOST", "standard-host")
		t.Setenv("DB_PORT", "5434")
		t.Setenv("DB_USERNAME", "standard-user")
		t.Setenv("DB_NAME", "standard-db")
		t.Setenv("DB_SSLMODE", "prefer")
		t.Setenv("DB_PASSWORD", "")

		connStr := getDatabaseConnectionString()
		expected := "host=standard-host port=5434 user=standard-user password= dbname=standard-db sslmode=prefer"
		assert.Equal(t, expected, connStr)
	})

	t.Run("No environment variables set (defaults)", func(t *testing.T) {
		t.Setenv("DB_HOST", "")
		t.Setenv("DB_PORT", "")
		t.Setenv("DB_USERNAME", "")
		t.Setenv("DB_PASSWORD", "")
		t.Setenv("DB_NAME", "")
		t.Setenv("DB_SSLMODE", "")

		connStr := getDatabaseConnectionString()
		expected := "host=localhost port=5432 user=postgres password= dbname=orchestration_engine sslmode=disable"
		assert.Equal(t, expected, connStr)
	})
}
