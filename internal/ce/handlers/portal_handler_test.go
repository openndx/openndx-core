package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/ce/middleware"
	"github.com/stretchr/testify/assert"
)

func portalTestMux(h *PortalHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", h.HealthCheck)
	mux.HandleFunc("GET /api/v1/consents/{consentId}", h.GetConsent)
	mux.HandleFunc("PUT /api/v1/consents/{consentId}", h.UpdateConsent)
	return mux
}

func newRequest(t *testing.T, method, path string, body []byte, ownerSubject string) *http.Request {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequestWithContext(context.Background(), method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequestWithContext(context.Background(), method, path, nil)
	}
	if ownerSubject != "" {
		req = req.WithContext(middleware.WithOwnerSubject(req.Context(), ownerSubject))
	}
	return req
}

func TestPortalHandler_HealthCheck(t *testing.T) {
	handler := &PortalHandler{consentService: nil}
	mux := portalTestMux(handler)

	req := newRequest(t, http.MethodGet, "/api/v1/health", nil, "")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", response["status"])
}

func TestPortalHandler_GetConsent_MissingConsentId(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := newRequest(t, http.MethodGet, "/api/v1/consents/", nil, "user-123")
	w := httptest.NewRecorder()
	handler.GetConsent(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPortalHandler_GetConsent_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := newRequest(t, http.MethodPost, "/api/v1/consents/"+uuid.New().String(), nil, "user-123")
	w := httptest.NewRecorder()
	handler.GetConsent(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPortalHandler_GetConsent_InvalidUUID(t *testing.T) {
	handler := &PortalHandler{consentService: nil}
	mux := portalTestMux(handler)

	req := newRequest(t, http.MethodGet, "/api/v1/consents/invalid-uuid", nil, "user-123")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid consentId format")
}

func TestPortalHandler_UpdateConsent_InvalidUUID(t *testing.T) {
	handler := &PortalHandler{consentService: nil}
	mux := portalTestMux(handler)

	req := newRequest(t, http.MethodPut, "/api/v1/consents/invalid-uuid", []byte(`{"action":"approve"}`), "user-123")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid consentId format")
}

func TestPortalHandler_UpdateConsent_InvalidAction(t *testing.T) {
	handler := &PortalHandler{consentService: nil}
	mux := portalTestMux(handler)

	consentID := uuid.New().String()
	req := newRequest(t, http.MethodPut, "/api/v1/consents/"+consentID, []byte(`{"action":"invalid"}`), "user-123")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid action")
}

func TestPortalHandler_UpdateConsent_MethodNotAllowed(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	consentID := uuid.New().String()
	req := newRequest(t, http.MethodGet, "/api/v1/consents/"+consentID, nil, "user-123")
	w := httptest.NewRecorder()
	handler.UpdateConsent(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPortalHandler_UpdateConsent_InvalidBody(t *testing.T) {
	handler := &PortalHandler{consentService: nil}
	mux := portalTestMux(handler)

	consentID := uuid.New().String()
	req := newRequest(t, http.MethodPut, "/api/v1/consents/"+consentID, []byte("invalid json"), "user-123")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid request body")
}

func TestPortalHandler_UpdateConsent_MissingConsentId(t *testing.T) {
	handler := &PortalHandler{consentService: nil}

	req := newRequest(t, http.MethodPut, "/api/v1/consents/", nil, "user-123")
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

	req := newRequest(t, http.MethodPost, "/api/v1/health", nil, "")
	w := httptest.NewRecorder()
	handler.HealthCheck(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
