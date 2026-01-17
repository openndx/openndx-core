package v1

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/OpenDIF/opendif-core/exchange/shared/utils"
	"github.com/gov-dx-sandbox/exchange/policy-decision-point/v1/models"
	"github.com/gov-dx-sandbox/exchange/policy-decision-point/v1/services"
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

	if len(parts) != 1 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	switch parts[0] {
	case "metadata":
		switch r.Method {
		case http.MethodPost:
			h.CreatePolicyMetadata(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	case "update-allowlist":
		switch r.Method {
		case http.MethodPost:
			h.UpdateAllowList(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	case "decide":
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

	resp, err := h.policyService.CreatePolicyMetadata(&req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.RespondWithSuccess(w, http.StatusCreated, resp)
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
