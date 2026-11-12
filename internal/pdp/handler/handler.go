package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/openndx/openndx-core/internal/pdp/fieldpolicy"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"github.com/openndx/openndx-core/internal/pdp/services"
	"github.com/openndx/openndx-core/internal/utils"
	"gorm.io/gorm"
)

// Handler handles all API requests
type Handler struct {
	policyService *services.PolicyMetadataService
}

// NewHandler creates a new API handler. fields reads the field policies that
// decisions are made on.
func NewHandler(db *gorm.DB, fields services.FieldReader) *Handler {
	policyService := services.NewPolicyMetadataService(db, fields)
	return &Handler{
		policyService: policyService,
	}
}

// CreateAccessGrants handles granting an application access to fields.
func (h *Handler) CreateAccessGrants(w http.ResponseWriter, r *http.Request) {
	var req models.AccessGrantCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := h.policyService.CreateAccessGrants(&req)
	if err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// GetAccessGrant handles reading one access grant.
func (h *Handler) GetAccessGrant(w http.ResponseWriter, r *http.Request) {
	resp, err := h.policyService.GetAccessGrant(r.PathValue("id"))
	if err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// ListAccessGrants handles listing access grants, filtered by application,
// schema, status or kind.
func (h *Handler) ListAccessGrants(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := models.AccessGrantFilter{
		ApplicationID: strings.TrimSpace(query.Get("applicationId")),
		SchemaID:      strings.TrimSpace(query.Get("schemaId")),
		Status:        models.GrantStatus(strings.TrimSpace(query.Get("status"))),
	}
	if value := strings.TrimSpace(query.Get("consentBypass")); value != "" {
		consentBypass, err := strconv.ParseBool(value)
		if err != nil {
			utils.RespondWithError(w, http.StatusBadRequest, "consentBypass must be true or false")
			return
		}
		filter.ConsentBypass = &consentBypass
	}

	resp, err := h.policyService.ListAccessGrants(filter)
	if err != nil {
		respondWithPolicyServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// RevokeAccessGrant handles revoking one active access grant.
func (h *Handler) RevokeAccessGrant(w http.ResponseWriter, r *http.Request) {
	resp, err := h.policyService.RevokeAccessGrant(r.PathValue("id"))
	if err != nil {
		respondWithPolicyServiceError(w, err)
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

	resp, err := h.policyService.GetPolicyDecision(r.Context(), &req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// policyServiceErrorStatuses maps the policy service's sentinel errors to HTTP
// statuses. Any other error is a 500.
var policyServiceErrorStatuses = []utils.ErrorStatus{
	{Err: fieldpolicy.ErrPolicyMetadataNotFound, Status: http.StatusNotFound},
	{Err: services.ErrAccessGrantNotFound, Status: http.StatusNotFound},
	{Err: services.ErrInvalidGrantRequest, Status: http.StatusBadRequest},
}

func respondWithPolicyServiceError(w http.ResponseWriter, err error) {
	utils.RespondWithServiceError(w, err, policyServiceErrorStatuses...)
}
