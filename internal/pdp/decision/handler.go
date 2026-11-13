package decision

import (
	"encoding/json"
	"net/http"

	"github.com/openndx/openndx-core/internal/utils"
)

// Handler handles the policy decision HTTP API.
type Handler struct {
	service *Service
}

// NewHandler creates a decision Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetPolicyDecision handles POST /api/v1/policy/decide.
func (h *Handler) GetPolicyDecision(w http.ResponseWriter, r *http.Request) {
	var req PolicyDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ApplicationID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "applicationId is required")
		return
	}
	if len(req.RequiredFields) == 0 {
		utils.RespondWithError(w, http.StatusBadRequest, "requiredFields is required and cannot be empty")
		return
	}

	// A field with no policy is reported as a 500, as it always has been.
	resp, err := h.service.GetPolicyDecision(r.Context(), &req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}
