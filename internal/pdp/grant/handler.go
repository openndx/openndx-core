package grant

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/openndx/openndx-core/internal/utils"
)

// Handler handles the access grant HTTP API
type Handler struct {
	service *Service
}

// NewHandler creates a new access grant Handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateAccessGrants handles POST /api/v1/policy/grants
func (h *Handler) CreateAccessGrants(w http.ResponseWriter, r *http.Request) {
	var req AccessGrantCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := h.service.CreateAccessGrants(&req)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// GetAccessGrant handles GET /api/v1/policy/grants/{id}
func (h *Handler) GetAccessGrant(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetAccessGrant(r.PathValue("id"))
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// ListAccessGrants handles GET /api/v1/policy/grants, filtered by
// application, schema, status or kind.
func (h *Handler) ListAccessGrants(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := AccessGrantFilter{
		ApplicationID: strings.TrimSpace(query.Get("applicationId")),
		SchemaID:      strings.TrimSpace(query.Get("schemaId")),
		Status:        GrantStatus(strings.TrimSpace(query.Get("status"))),
	}
	if value := strings.TrimSpace(query.Get("consentBypass")); value != "" {
		consentBypass, err := strconv.ParseBool(value)
		if err != nil {
			utils.RespondWithError(w, http.StatusBadRequest, "consentBypass must be true or false")
			return
		}
		filter.ConsentBypass = &consentBypass
	}

	resp, err := h.service.ListAccessGrants(filter)
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// RevokeAccessGrant handles POST /api/v1/policy/grants/{id}/revoke
func (h *Handler) RevokeAccessGrant(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.RevokeAccessGrant(r.PathValue("id"))
	if err != nil {
		respondWithServiceError(w, err)
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, resp)
}

// serviceErrorStatuses maps the access grant service's sentinel errors to HTTP
// statuses. Any other error is a 500.
var serviceErrorStatuses = []utils.ErrorStatus{
	{Err: ErrFieldNotFound, Status: http.StatusNotFound},
	{Err: ErrAccessGrantNotFound, Status: http.StatusNotFound},
	{Err: ErrInvalidGrantRequest, Status: http.StatusBadRequest},
}

func respondWithServiceError(w http.ResponseWriter, err error) {
	utils.RespondWithServiceError(w, err, serviceErrorStatuses...)
}
