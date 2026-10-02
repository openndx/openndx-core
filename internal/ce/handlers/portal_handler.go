package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/ce/middleware"
	"github.com/openndx/openndx-core/internal/ce/models"
	"github.com/openndx/openndx-core/internal/ce/services"
	"github.com/openndx/openndx-core/internal/ce/utils"
)

// PortalHandler handles external API requests (authentication required)
type PortalHandler struct {
	consentService *services.ConsentService
}

// NewPortalHandler creates a new portal handler
func NewPortalHandler(consentService *services.ConsentService) *PortalHandler {
	return &PortalHandler{
		consentService: consentService,
	}
}

// HealthCheck handles GET /api/v1/health
func (h *PortalHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.RespondWithError(w, http.StatusMethodNotAllowed, models.ErrorCodeMethodNotAllowed, "Method not allowed")
		return
	}

	response := map[string]string{
		"status": "healthy",
	}
	utils.RespondWithJSON(w, http.StatusOK, response)
}

// ListConsents handles GET /api/v1/consents
// Authorization: Bearer Token
// Lists the consents owned by the authenticated user's subject (UID), newest first.
// Query: status (repeatable or comma-separated), limit, offset
func (h *PortalHandler) ListConsents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.RespondWithError(w, http.StatusMethodNotAllowed, models.ErrorCodeMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract owner subject (UID) from request context (set by auth middleware)
	ownerSubject, ok := middleware.GetOwnerSubjectFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Owner identifier not found in token")
		return
	}

	query := r.URL.Query()

	statuses, err := parseStatusFilter(query["status"])
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, err.Error())
		return
	}

	limit, err := parseNonNegativeIntParam(query.Get("limit"), models.DefaultConsentListLimit)
	if err != nil || limit < 1 || limit > models.MaxConsentListLimit {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, fmt.Sprintf("limit must be an integer between 1 and %d", models.MaxConsentListLimit))
		return
	}

	offset, err := parseNonNegativeIntParam(query.Get("offset"), 0)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "offset must be a non-negative integer")
		return
	}

	consents, err := h.consentService.ListConsentsByOwner(r.Context(), ownerSubject, statuses, limit, offset)
	if err != nil {
		// Check if error is due to context cancellation or timeout
		if r.Context().Err() != nil {
			slog.Warn("Request context cancelled during service call", "error", r.Context().Err())
			utils.RespondWithError(w, http.StatusRequestTimeout, models.ErrorCodeInternalError, "Request timeout or cancelled")
			return
		}
		slog.Error("Failed to list consents", "error", err)
		utils.RespondWithError(w, http.StatusInternalServerError, models.ErrorCodeInternalError, "An unexpected error occurred")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, consents)
}

// parseStatusFilter accepts repeated and/or comma-separated status values and validates each one
func parseStatusFilter(values []string) ([]models.ConsentStatus, error) {
	var statuses []models.ConsentStatus
	for _, value := range values {
		for raw := range strings.SplitSeq(value, ",") {
			status := models.ConsentStatus(strings.TrimSpace(raw))
			if status == "" {
				continue
			}
			switch status {
			case models.StatusPending, models.StatusApproved, models.StatusRejected, models.StatusExpired, models.StatusRevoked:
				statuses = append(statuses, status)
			default:
				return nil, fmt.Errorf("invalid status: %s", status)
			}
		}
	}
	return statuses, nil
}

// parseNonNegativeIntParam parses an optional non-negative integer query parameter
func parseNonNegativeIntParam(value string, defaultValue int) (int, error) {
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid value: %s", value)
	}
	return parsed, nil
}

// GetConsent handles GET /api/v1/consents/:consentId
// Authorization: Bearer Token
// Verifies that consent.owner_id matches the subject (UID) from the decoded token
func (h *PortalHandler) GetConsent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.RespondWithError(w, http.StatusMethodNotAllowed, models.ErrorCodeMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract consentId from URL path parameter
	consentID := r.PathValue("consentId")
	if consentID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "consentId is required")
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(consentID); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "invalid consentId format")
		return
	}

	// Extract owner subject (UID) from request context (set by auth middleware)
	ownerSubject, ok := middleware.GetOwnerSubjectFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Owner identifier not found in token")
		return
	}

	// Get consent from service; the service verifies the owner matches the authenticated subject (UID)
	consent, err := h.consentService.GetConsentPortalView(r.Context(), consentID, ownerSubject)
	if err != nil {
		// Check if error is due to context cancellation or timeout
		if r.Context().Err() != nil {
			slog.Warn("Request context cancelled during service call", "error", r.Context().Err())
			utils.RespondWithError(w, http.StatusRequestTimeout, models.ErrorCodeInternalError, "Request timeout or cancelled")
			return
		}
		if errors.Is(err, models.ErrConsentNotFound) {
			utils.RespondWithError(w, http.StatusNotFound, models.ErrorCodeConsentNotFound, "Consent not found")
			return
		}
		if errors.Is(err, models.ErrConsentAccessDenied) {
			utils.RespondWithError(w, http.StatusForbidden, models.ErrorCodeForbidden, "Access denied: consent belongs to a different user")
			return
		}
		slog.Error("Failed to get consent", "error", err)
		utils.RespondWithError(w, http.StatusInternalServerError, models.ErrorCodeInternalError, "An unexpected error occurred")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, consent)
}

// UpdateConsent handles PUT /api/v1/consents/:consentId
// Authorization: Bearer Token
// Verifies that consent.owner_id matches the subject (UID) from the decoded token
// Body: { "action": "approve" | "reject" }
func (h *PortalHandler) UpdateConsent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		utils.RespondWithError(w, http.StatusMethodNotAllowed, models.ErrorCodeMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract consentId from URL path parameter
	consentID := r.PathValue("consentId")
	if consentID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "consentId is required")
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(consentID); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "invalid consentId format")
		return
	}

	// Extract owner subject (UID) from request context (set by auth middleware)
	ownerSubject, ok := middleware.GetOwnerSubjectFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, models.ErrorCodeUnauthorized, "Owner identifier not found in token")
		return
	}

	// Parse request body
	var actionReq struct {
		Action string `json:"action"`
	}
	defer func() { _ = r.Body.Close() }()
	if err := json.NewDecoder(r.Body).Decode(&actionReq); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate action
	if actionReq.Action != string(models.ActionApprove) && actionReq.Action != string(models.ActionReject) {
		utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, fmt.Sprintf("Invalid action: %s. Must be 'approve' or 'reject'", actionReq.Action))
		return
	}

	// Update consent status; the service verifies the owner matches the authenticated subject (UID)
	updateReq := models.ConsentPortalActionRequest{
		ConsentID: consentID,
		OwnerID:   ownerSubject,
		Action:    models.ConsentPortalAction(actionReq.Action),
		UpdatedBy: ownerSubject,
	}

	if err := h.consentService.UpdateConsentStatusByPortalAction(r.Context(), updateReq); err != nil {
		// Check if error is due to context cancellation or timeout
		if r.Context().Err() != nil {
			slog.Warn("Request context cancelled during update operation", "error", r.Context().Err())
			utils.RespondWithError(w, http.StatusRequestTimeout, models.ErrorCodeInternalError, "Request timeout or cancelled")
			return
		}
		if errors.Is(err, models.ErrConsentNotFound) {
			utils.RespondWithError(w, http.StatusNotFound, models.ErrorCodeConsentNotFound, "Consent not found")
			return
		}
		if errors.Is(err, models.ErrConsentAccessDenied) {
			utils.RespondWithError(w, http.StatusForbidden, models.ErrorCodeForbidden, "Access denied: consent belongs to a different user")
			return
		}
		if errors.Is(err, models.ErrConsentNotPending) {
			utils.RespondWithError(w, http.StatusConflict, models.ErrorCodeConsentNotPending, "Consent is no longer pending and cannot be updated")
			return
		}
		if errors.Is(err, models.ErrPortalRequestFailed) {
			utils.RespondWithError(w, http.StatusBadRequest, models.ErrorCodeBadRequest, "Invalid consent update request")
			return
		}
		slog.Error("Failed to update consent", "error", err)
		utils.RespondWithError(w, http.StatusInternalServerError, models.ErrorCodeInternalError, "An unexpected error occurred")
		return
	}

	// Return success response with actual consent status
	statusMap := map[string]string{
		string(models.ActionApprove): string(models.StatusApproved),
		string(models.ActionReject):  string(models.StatusRejected),
	}
	response := map[string]string{
		"message": "Consent updated successfully",
		"status":  statusMap[actionReq.Action],
	}
	utils.RespondWithJSON(w, http.StatusOK, response)
}
