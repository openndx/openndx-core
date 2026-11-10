package consentbypass

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/openndx/openndx-core/internal/utils"
	"gorm.io/gorm"
)

// Handler serves the consent bypass API.
type Handler struct {
	service *Service
}

// NewHandler creates a consent bypass Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Grant handles POST /api/v1/applications/{applicationId}/consent-bypass-grants.
// The route requires consent-bypass:write. The audit event records who
// granted the bypass.
func (h *Handler) Grant(w http.ResponseWriter, r *http.Request) {
	applicationID := r.PathValue("applicationId")

	var req GrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	grants, err := h.service.Grant(r.Context(), applicationID, &req)
	if err != nil {
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeConsentBypassGrants), &applicationID, string(middleware.AuditStatusFailure))
		respondWithError(w, err)
		return
	}

	for i := range grants {
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeConsentBypassGrants), &grants[i].ID, string(middleware.AuditStatusSuccess))
	}

	utils.RespondWithSuccess(w, http.StatusOK, kernel.CollectionResponse{Items: grants, Count: len(grants)})
}

// List handles GET /api/v1/consent-bypass-grants. The route requires
// consent-bypass:read.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	grants, err := h.service.List(r.Context(),
		strings.TrimSpace(query.Get("applicationId")), strings.TrimSpace(query.Get("status")))
	if err != nil {
		respondWithError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, kernel.CollectionResponse{Items: grants, Count: len(grants)})
}

// Revoke handles POST /api/v1/consent-bypass-grants/{grantId}/revoke. The
// route requires consent-bypass:write.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	grantID := r.PathValue("grantId")

	grant, err := h.service.Revoke(r.Context(), grantID)
	if err != nil {
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeConsentBypassGrants), &grantID, string(middleware.AuditStatusFailure))
		respondWithError(w, err)
		return
	}

	middleware.LogAuditEvent(r, string(middleware.ResourceTypeConsentBypassGrants), &grantID, string(middleware.AuditStatusSuccess))
	utils.RespondWithSuccess(w, http.StatusOK, grant)
}

// respondWithError maps consent bypass errors onto a response. Client errors
// reported by the PDP, such as a field that asks for no consent, are passed
// through; PDP failures are reported as a bad gateway.
func respondWithError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.RespondWithError(w, http.StatusNotFound, "Application not found")
		return
	case errors.Is(err, ErrGrantNotFound):
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrInvalidRequest):
		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	var pdpErr *policy.Error
	if errors.As(err, &pdpErr) {
		switch pdpErr.StatusCode {
		case http.StatusBadRequest, http.StatusNotFound:
			utils.RespondWithError(w, pdpErr.StatusCode, pdpErr.Message)
			return
		}
		slog.Error("Consent bypass request to PDP failed", "error", err)
		utils.RespondWithError(w, http.StatusBadGateway, "Policy decision point request failed")
		return
	}

	slog.Error("Consent bypass request failed", "error", err)
	utils.RespondWithError(w, http.StatusInternalServerError, "internal server error")
}
