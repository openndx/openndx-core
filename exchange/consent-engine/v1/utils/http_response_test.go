package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/models"
	"github.com/stretchr/testify/assert"
)

func TestRespondWithJSON(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]string{"status": "healthy"}

	RespondWithJSON(w, http.StatusOK, payload)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
}

func TestRespondWithJSON_ComplexPayload(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"id":    "123",
		"name":  "test",
		"items": []string{"a", "b", "c"},
	}

	RespondWithJSON(w, http.StatusCreated, payload)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "123", response["id"])
	assert.Equal(t, "test", response["name"])
}

func TestRespondWithError(t *testing.T) {
	w := httptest.NewRecorder()

	RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "Invalid request")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, string(models.ErrorCodeBadRequest), response.Error.Code)
	assert.Equal(t, "Invalid request", response.Error.Message)
}

func TestRespondWithError_Unauthorized(t *testing.T) {
	w := httptest.NewRecorder()

	RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Token expired")

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, string(models.ErrorCodeUnauthorized), response.Error.Code)
	assert.Equal(t, "Token expired", response.Error.Message)
}

func TestRespondWithError_InternalError(t *testing.T) {
	w := httptest.NewRecorder()

	RespondWithError(w, http.StatusInternalServerError, models.ErrorCodeInternalError, "Database connection failed")

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, string(models.ErrorCodeInternalError), response.Error.Code)
	assert.Equal(t, "Database connection failed", response.Error.Message)
}

func TestRespondWithError_Forbidden(t *testing.T) {
	w := httptest.NewRecorder()

	RespondWithError(w, http.StatusForbidden, models.ErrorCodeForbidden, "Access denied")

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, string(models.ErrorCodeForbidden), response.Error.Code)
	assert.Equal(t, "Access denied", response.Error.Message)
}

func TestRespondWithError_NotFound(t *testing.T) {
	w := httptest.NewRecorder()

	RespondWithError(w, http.StatusNotFound, models.ErrorCodeConsentNotFound, "Consent not found")

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, string(models.ErrorCodeConsentNotFound), response.Error.Code)
	assert.Equal(t, "Consent not found", response.Error.Message)
}

// TestRespondWithJSON_EncodingError tests the error path when JSON encoding fails
// This tests the error handling in RespondWithJSON when json.Encode returns an error
func TestRespondWithJSON_EncodingError(t *testing.T) {
	w := httptest.NewRecorder()

	// Use a channel as payload - channels cannot be JSON encoded, which will cause an error
	ch := make(chan int)
	RespondWithJSON(w, http.StatusOK, ch)

	// Status code should still be set even if encoding fails
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	// Body should be empty or contain error indication
	// The function logs the error but doesn't write to response body after headers are written
}
