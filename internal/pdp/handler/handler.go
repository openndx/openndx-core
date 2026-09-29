package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/services"
	"github.com/openndx/openndx-core/internal/utils"
	"gorm.io/gorm"
)

// Handler handles all API requests
type Handler struct {
	policyService *services.PolicyMetadataService
}

// NewHandler creates a new API handler
func NewHandler(db *gorm.DB) *Handler {
	policyService := services.NewPolicyMetadataService(db)
	return &Handler{
		policyService: policyService,
	}
}

// CreatePolicyMetadata handles creating policy metadata
func (h *Handler) CreatePolicyMetadata(w http.ResponseWriter, r *http.Request) {
	var req models.PolicyMetadataCreateRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.SchemaID) == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "schemaId is required and cannot be empty")
		return
	}

	resp, err := h.policyService.CreatePolicyMetadata(&req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusCreated, resp)
}

// ListPolicyMetadata handles listing policy metadata with an optional schema filter.
func (h *Handler) ListPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaID := strings.TrimSpace(r.URL.Query().Get("schemaId"))

	resp, err := h.policyService.ListPolicyMetadata(schemaID)
	if err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// PatchPolicyMetadata handles partially updating one policy metadata record.
func (h *Handler) PatchPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req models.PolicyMetadataPatchRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.IsEmpty() {
		utils.RespondWithError(w, http.StatusBadRequest, "at least one editable field is required")
		return
	}

	if req.AccessControlType.Set {
		if req.AccessControlType.Value == nil {
			utils.RespondWithError(w, http.StatusBadRequest, "accessControlType cannot be null")
			return
		}

		accessControlType := *req.AccessControlType.Value
		if accessControlType != models.AccessControlTypePublic &&
			accessControlType != models.AccessControlTypeRestricted {
			utils.RespondWithError(w, http.StatusBadRequest, "accessControlType must be public or restricted")
			return
		}
	}

	resp, err := h.policyService.PatchPolicyMetadata(id, &req)
	if err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// DeletePolicyMetadata handles deleting one policy metadata record.
func (h *Handler) DeletePolicyMetadata(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.policyService.DeletePolicyMetadata(id); err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RevokeAllowListEntry handles removing one application from a field's allow-list.
func (h *Handler) RevokeAllowListEntry(
	w http.ResponseWriter,
	r *http.Request,
) {
	id := r.PathValue("id")

	applicationID := strings.TrimSpace(r.PathValue("applicationId"))
	if applicationID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "applicationId is required")
		return
	}

	if err := h.policyService.RevokeAllowListEntry(id, applicationID); err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateAllowList handles updating the allow list for a policy
func (h *Handler) UpdateAllowList(w http.ResponseWriter, r *http.Request) {
	var req models.AllowListUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := h.policyService.UpdateAllowList(&req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
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

	resp, err := h.policyService.GetPolicyDecision(&req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

func respondWithPolicyServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrPolicyMetadataNotFound):
		utils.RespondWithError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, services.ErrAllowListEntryNotFound):
		utils.RespondWithError(w, http.StatusNotFound, err.Error())

	default:
		slog.Error("Policy service request failed", "error", err)
		utils.RespondWithError(w, http.StatusInternalServerError, "internal server error")
	}
}
