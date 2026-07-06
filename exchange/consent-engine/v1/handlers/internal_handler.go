package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/models"
	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/services"
	"github.com/gov-dx-sandbox/exchange/consent-engine/v1/utils"
)

// InternalHandler handles internal API requests (no authentication required)
type InternalHandler struct {
	consentService *services.ConsentService
}

// NewInternalHandler creates a new internal handler
func NewInternalHandler(consentService *services.ConsentService) *InternalHandler {
	return &InternalHandler{
		consentService: consentService,
	}
}

// HealthCheck handles GET /internal/api/v1/health
func (h *InternalHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.RespondWithError(w, http.StatusMethodNotAllowed, models.ErrorCodeMethodNotAllowed, "Method not allowed")
		return
	}

	response := map[string]string{
		"status": "healthy",
	}
	utils.RespondWithJSON(w, http.StatusOK, response)
}

// GetConsent handles GET /internal/api/v1/consents
// Query parameters: ownerEmail & appId OR ownerId & appId
// Returns: models.ConsentResponseInternalView
func (h *InternalHandler) GetConsent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.RespondWithError(w, http.StatusMethodNotAllowed, models.ErrorCodeMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse query parameters
	ownerEmail := r.URL.Query().Get("ownerEmail")
	ownerID := r.URL.Query().Get("ownerId")
	appID := r.URL.Query().Get("appId")

	// Validate required parameters
	if appID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "appId is required")
		return
	}

	if ownerEmail == "" && ownerID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "either ownerEmail or ownerId is required")
		return
	}

	// Get consent from service (context with timeout is propagated)
	var consent *models.ConsentResponseInternalView
	var err error

	if ownerEmail != "" {
		consent, err = h.consentService.GetConsentInternalView(r.Context(), nil, nil, &ownerEmail, &appID)
	} else {
		consent, err = h.consentService.GetConsentInternalView(r.Context(), nil, &ownerID, nil, &appID)
	}

	if err != nil {
		// Check if error is due to context cancellation or timeout
		if r.Context().Err() != nil {
			slog.Warn("Request context cancelled during service call", "error", r.Context().Err())
			utils.RespondWithError(w, http.StatusRequestTimeout, models.ErrorCodeInternalError, "Request timeout or cancelled")
			return
		}
		if errors.Is(err, models.ErrConsentNotFound) {
			utils.RespondWithError(w, http.StatusNotFound, models.ErrorCodeConsentNotFound, err.Error())
			return
		}
		slog.Error("Failed to get consent", "error", err)
		utils.RespondWithError(w, http.StatusInternalServerError, models.ErrorCodeInternalError, "An unexpected error occurred")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, consent)
}

// CreateConsent handles POST /internal/api/v1/consents
// Body: models.CreateConsentRequest
// Returns: []models.ConsentResponseInternalView
func (h *InternalHandler) CreateConsent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.RespondWithError(w, http.StatusMethodNotAllowed, models.ErrorCodeMethodNotAllowed, "Method not allowed")
		return
	}

	defer func() { _ = r.Body.Close() }()
	// Parse request body
	var req models.CreateConsentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Create consent records (context with timeout is propagated)
	consents, err := h.consentService.CreateConsentRecord(r.Context(), req)
	if err != nil {
		// Check if error is due to context cancellation or timeout
		if r.Context().Err() != nil {
			slog.Warn("Request context cancelled during service call", "error", r.Context().Err())
			utils.RespondWithError(w, http.StatusRequestTimeout, models.ErrorCodeInternalError, "Request timeout or cancelled")
			return
		}
		if errors.Is(err, models.ErrConsentCreateFailed) {
			slog.Error("Failed to create consent", "error", err)
			utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, err.Error())
			return
		}
		slog.Error("Failed to create consent", "error", err)
		utils.RespondWithError(w, http.StatusInternalServerError, models.ErrorCodeInternalError, "An unexpected error occurred")
		return
	}

	utils.RespondWithJSON(w, http.StatusCreated, consents)
}
