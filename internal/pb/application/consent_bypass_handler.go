package application

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

// GrantConsentBypass handles POST /api/v1/applications/{applicationId}/consent-bypass-grants.
// The route requires consent-bypass:write. The audit event records who
// granted the bypass.
func (h *Handler) GrantConsentBypass(w http.ResponseWriter, r *http.Request) {
	applicationID := r.PathValue("applicationId")

	var req ConsentBypassRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	grants, err := h.service.GrantConsentBypass(r.Context(), applicationID, &req)
	if err != nil {
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeConsentBypassGrants), &applicationID, string(middleware.AuditStatusFailure))
		respondWithConsentBypassError(w, err)
		return
	}

	for i := range grants {
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeConsentBypassGrants), &grants[i].ID, string(middleware.AuditStatusSuccess))
	}

	utils.RespondWithSuccess(w, http.StatusOK, kernel.CollectionResponse{Items: grants, Count: len(grants)})
}

// ListConsentBypassGrants handles GET /api/v1/consent-bypass-grants. The route
// requires consent-bypass:read.
func (h *Handler) ListConsentBypassGrants(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	grants, err := h.service.ListConsentBypassGrants(r.Context(),
		strings.TrimSpace(query.Get("applicationId")), strings.TrimSpace(query.Get("status")))
	if err != nil {
		respondWithConsentBypassError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, kernel.CollectionResponse{Items: grants, Count: len(grants)})
}

// RevokeConsentBypass handles POST /api/v1/consent-bypass-grants/{grantId}/revoke.
// The route requires consent-bypass:write.
func (h *Handler) RevokeConsentBypass(w http.ResponseWriter, r *http.Request) {
	grantID := r.PathValue("grantId")

	grant, err := h.service.RevokeConsentBypass(r.Context(), grantID)
	if err != nil {
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeConsentBypassGrants), &grantID, string(middleware.AuditStatusFailure))
		respondWithConsentBypassError(w, err)
		return
	}

	middleware.LogAuditEvent(r, string(middleware.ResourceTypeConsentBypassGrants), &grantID, string(middleware.AuditStatusSuccess))
	utils.RespondWithSuccess(w, http.StatusOK, grant)
}

// respondWithConsentBypassError maps consent bypass errors onto a response.
// Client errors reported by the PDP, such as a field that asks for no
// consent, are passed through; PDP failures are reported as a bad gateway.
func respondWithConsentBypassError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.RespondWithError(w, http.StatusNotFound, "Application not found")
		return
	case errors.Is(err, ErrConsentBypassGrantNotFound):
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrInvalidConsentBypassRequest):
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
