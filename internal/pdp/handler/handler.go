package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
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

// SetupRoutes configures all API routes
func (h *Handler) SetupRoutes(mux *http.ServeMux) {
	mux.Handle("/api/v1/policy/", utils.PanicRecoveryMiddleware(http.HandlerFunc(h.handlePolicyService)))
}

// handlePolicyService handles policy metadata service requests
func (h *Handler) handlePolicyService(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/policy")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	switch parts[0] {
	case "metadata":
		switch len(parts) {
		case 1:
			switch r.Method {
			case http.MethodGet:
				h.ListPolicyMetadata(w, r)
			case http.MethodPost:
				h.CreatePolicyMetadata(w, r)
			default:
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}

		case 2:
			switch r.Method {
			case http.MethodPatch:
				h.PatchPolicyMetadata(w, r, parts[1])
			case http.MethodDelete:
				h.DeletePolicyMetadata(w, r, parts[1])
			default:
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}

		case 4:
			if parts[2] != "allowlist" {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}

			switch r.Method {
			case http.MethodDelete:
				h.RevokeAllowListEntry(w, r, parts[1], parts[3])
			default:
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}

		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}

	case "update-allowlist":
		if len(parts) != 1 {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		switch r.Method {
		case http.MethodPost:
			h.UpdateAllowList(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}

	case "decide":
		if len(parts) != 1 {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		switch r.Method {
		case http.MethodPost:
			h.GetPolicyDecision(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}

	default:
		http.Error(w, "Not Found", http.StatusNotFound)
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

// ListPolicyMetadata handles listing policy metadata for one schema.
func (h *Handler) ListPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaID := strings.TrimSpace(r.URL.Query().Get("schemaId"))
	if schemaID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "schemaId is required and cannot be empty")
		return
	}

	resp, err := h.policyService.ListPolicyMetadata(schemaID)
	if err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// PatchPolicyMetadata handles partially updating one policy metadata record.
func (h *Handler) PatchPolicyMetadata(w http.ResponseWriter, r *http.Request, idValue string) {
	id, ok := parsePolicyMetadataID(w, idValue)
	if !ok {
		return
	}

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
func (h *Handler) DeletePolicyMetadata(w http.ResponseWriter, _ *http.Request, idValue string) {
	id, ok := parsePolicyMetadataID(w, idValue)
	if !ok {
		return
	}

	if err := h.policyService.DeletePolicyMetadata(id); err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RevokeAllowListEntry handles removing one application from a field's allow-list.
func (h *Handler) RevokeAllowListEntry(
	w http.ResponseWriter,
	_ *http.Request,
	idValue string,
	applicationID string,
) {
	id, ok := parsePolicyMetadataID(w, idValue)
	if !ok {
		return
	}

	applicationID = strings.TrimSpace(applicationID)
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

// parsePolicyMetadataID parses and validates a policy metadata UUID.
func parsePolicyMetadataID(w http.ResponseWriter, idValue string) (uuid.UUID, bool) {
	id, err := uuid.Parse(idValue)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "invalid policy metadata id")
		return uuid.Nil, false
	}

	return id, true
}

func respondWithPolicyServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrPolicyMetadataNotFound):
		utils.RespondWithError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, services.ErrAllowListEntryNotFound):
		utils.RespondWithError(w, http.StatusNotFound, err.Error())

	default:
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
	}
}
