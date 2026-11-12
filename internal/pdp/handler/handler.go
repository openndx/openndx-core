package handler

import (
	"encoding/json"
	"net/http"

	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/services"
	"github.com/openndx/openndx-core/internal/utils"
)

// Handler handles policy decision requests
type Handler struct {
	policyService *services.PolicyMetadataService
}

// NewHandler creates a new API handler. fields reads the field policies and
// grants the access grants that decisions are made on.
func NewHandler(fields services.FieldReader, grants services.GrantReader) *Handler {
	return &Handler{
		policyService: services.NewPolicyMetadataService(fields, grants),
	}
}

// GetPolicyDecision handles getting a policy decision
func (h *Handler) GetPolicyDecision(w http.ResponseWriter, r *http.Request) {
	var req models.PolicyDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.ApplicationID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "applicationId is required")
		return
	}
	if len(req.RequiredFields) == 0 {
		utils.RespondWithError(w, http.StatusBadRequest, "requiredFields is required and cannot be empty")
		return
	}

	resp, err := h.policyService.GetPolicyDecision(r.Context(), &req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}
