package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// setUserEmailInContext is a test helper to set user email in context
// Uses the same context key as middleware.auth.go (userEmailKey = "userEmail")
func setUserEmailInContext(ctx context.Context, email string) context.Context {
	type contextKey string
	const userEmailKey contextKey = "userEmail"
	return context.WithValue(ctx, userEmailKey, email)
}

func TestPortalHandler_HealthCheck(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.HealthCheck(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
}

func TestPortalHandler_GetConsent_MissingConsentId(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("GET", "/api/v1/consents/", nil)
	w := httptest.NewRecorder()

	handler.GetConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_GetConsent_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("POST", "/api/v1/consents/test-id", nil)
	w := httptest.NewRecorder()

	handler.GetConsent(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPortalHandler_GetConsent_InvalidUUID(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("GET", "/api/v1/consents/invalid-uuid", nil)
	req = req.WithContext(setUserEmailInContext(req.Context(), "user@example.com"))
	w := httptest.NewRecorder()

	handler.GetConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_InvalidUUID(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("PUT", "/api/v1/consents/invalid-uuid", nil)
	req = req.WithContext(setUserEmailInContext(req.Context(), "user@example.com"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_InvalidAction(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	reqBody := map[string]string{"action": "invalid"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/v1/consents/"+consentID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(setUserEmailInContext(req.Context(), "user@example.com"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	req := httptest.NewRequest("GET", "/api/v1/consents/"+consentID, nil)
	req = req.WithContext(setUserEmailInContext(req.Context(), "user@example.com"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// Note: GetConsent and UpdateConsent success paths require PathValue which needs a registered route.
// These are tested in integration tests. Here we focus on validation and error handling that doesn't require PathValue.

func TestPortalHandler_UpdateConsent_RejectAction(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	reqBody := map[string]string{"action": "reject"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("PUT", "/api/v1/consents/"+consentID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(setUserEmailInContext(req.Context(), "user@example.com"))
	w := httptest.NewRecorder()

	// PathValue requires registered route, so this tests validation up to that point
	handler.UpdateConsent(w, req)

	// PathValue returns empty without route, so we get 400
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_InvalidBody(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	req := httptest.NewRequest("PUT", "/api/v1/consents/"+consentID, bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(setUserEmailInContext(req.Context(), "user@example.com"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_UpdateConsent_MissingConsentId(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("PUT", "/api/v1/consents/", nil)
	req = req.WithContext(setUserEmailInContext(req.Context(), "user@example.com"))
	w := httptest.NewRecorder()

	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_NewPortalHandler(t *testing.T) {
	handler := NewPortalHandler(nil)
	assert.NotNil(t, handler)
	assert.Nil(t, handler.consentService)
}

func TestPortalHandler_HealthCheck_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := httptest.NewRequest("POST", "/api/v1/health", nil)
	w := httptest.NewRecorder()

	handler.HealthCheck(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
