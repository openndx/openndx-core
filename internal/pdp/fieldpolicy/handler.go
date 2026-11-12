package fieldpolicy

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/openndx/openndx-core/internal/utils"
)

// Handler handles the field policy HTTP API
type Handler struct {
	service *Service
}

// NewHandler creates a new field policy Handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreatePolicyMetadata handles POST /api/v1/policy/metadata
func (h *Handler) CreatePolicyMetadata(w http.ResponseWriter, r *http.Request) {
	var req PolicyMetadataCreateRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.SchemaID) == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "schemaId is required and cannot be empty")
		return
	}

	resp, err := h.service.CreatePolicyMetadata(&req)
	if errors.Is(err, ErrInvalidPolicy) {
		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusCreated, resp)
}

// ListPolicyMetadata handles GET /api/v1/policy/metadata, with an optional
// schemaId filter.
func (h *Handler) ListPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaID := strings.TrimSpace(r.URL.Query().Get("schemaId"))

	resp, err := h.service.ListPolicyMetadata(schemaID)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// PatchPolicyMetadata handles PATCH /api/v1/policy/metadata/{id}
func (h *Handler) PatchPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req PolicyMetadataPatchRequest
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

	if flag := req.NullFlag(); flag != "" {
		utils.RespondWithError(w, http.StatusBadRequest, flag+" cannot be null")
		return
	}

	resp, err := h.service.PatchPolicyMetadata(id, &req)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// DeletePolicyMetadata handles DELETE /api/v1/policy/metadata/{id}
func (h *Handler) DeletePolicyMetadata(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.service.DeletePolicyMetadata(id); err != nil {
		respondWithServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// serviceErrorStatuses maps the field policy service's sentinel errors to HTTP
// statuses. Any other error is a 500.
var serviceErrorStatuses = []utils.ErrorStatus{
	{Err: ErrPolicyMetadataNotFound, Status: http.StatusNotFound},
	{Err: ErrInvalidPolicy, Status: http.StatusBadRequest},
}

func respondWithServiceError(w http.ResponseWriter, err error) {
	utils.RespondWithServiceError(w, err, serviceErrorStatuses...)
}
