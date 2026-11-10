package schema

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/openndx/openndx-core/internal/pb/caller"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/openndx/openndx-core/internal/utils"
)

// Handler handles the schema, schema submission and schema policy metadata HTTP API
type Handler struct {
	service *Service
}

// NewHandler creates a new schema Handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Schema submission handlers

// GetAllSchemaSubmissions handles GET /api/v1/schema-submissions
func (h *Handler) GetAllSchemaSubmissions(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")
	statusFilter := r.URL.Query()["status"]

	// Unless the caller reads across organization units, limit it to its own submissions
	if !caller.HasScope(r.Context(), caller.ScopeSchemaSubmissionReadAll) {
		callerMemberID, ok := caller.MemberID(r.Context())
		if !ok {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		memberId = callerMemberID
	}

	submissions, err := h.service.GetSchemaSubmissions(&memberId, &statusFilter)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := kernel.CollectionResponse{
		Items: submissions,
		Count: len(submissions),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetSchemaSubmission handles GET /api/v1/schema-submissions/{submissionId}
func (h *Handler) GetSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	submission, err := h.service.GetSchemaSubmission(submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeSchemaSubmissionReadAll, submission.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// CreateSchemaSubmission handles POST /api/v1/schema-submissions
func (h *Handler) CreateSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	var req CreateSchemaSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Unless the caller writes across organization units, ensure they can only create submissions for themselves
	if !caller.HasScope(r.Context(), caller.ScopeSchemaSubmissionWriteAll) {
		callerMemberID, ok := caller.MemberID(r.Context())
		if !ok {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}

		// A provided MemberID must be the caller's own; an omitted one defaults to it
		if req.MemberID != "" && req.MemberID != callerMemberID {
			utils.RespondWithError(w, http.StatusForbidden, "Access denied: cannot create submission for another user")
			return
		}
		req.MemberID = callerMemberID
	}

	submission, err := h.service.CreateSchemaSubmission(&req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemaSubmissions), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemaSubmissions), &submission.SubmissionID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, submission)
}

// UpdateSchemaSubmission handles PUT /api/v1/schema-submissions/{submissionId}
func (h *Handler) UpdateSchemaSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get existing submission to check ownership
	existingSubmission, err := h.service.GetSchemaSubmission(submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeSchemaSubmissionWriteAll, existingSubmission.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
		return
	}

	var req UpdateSchemaSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	submission, err := h.service.UpdateSchemaSubmission(submissionId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemaSubmissions), &existingSubmission.SubmissionID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemaSubmissions), &submission.SubmissionID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// Schema handlers

// GetAllSchemas handles GET /api/v1/schemas
func (h *Handler) GetAllSchemas(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")

	// Unless the caller reads across organization units, limit it to its own schemas
	if !caller.HasScope(r.Context(), caller.ScopeSchemaReadAll) {
		callerMemberID, ok := caller.MemberID(r.Context())
		if !ok {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		memberId = callerMemberID
	}

	schemas, err := h.service.GetSchemas(&memberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := kernel.CollectionResponse{
		Items: schemas,
		Count: len(schemas),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetSchema handles GET /api/v1/schemas/{schemaId}
func (h *Handler) GetSchema(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")

	schema, err := h.service.GetSchema(schemaId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeSchemaReadAll, schema.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, schema)
}

// CreateSchema handles POST /api/v1/schemas
func (h *Handler) CreateSchema(w http.ResponseWriter, r *http.Request) {
	var req CreateSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Unless the caller writes across organization units, it creates schemas for its own member
	if !caller.HasScope(r.Context(), caller.ScopeSchemaWriteAll) {
		callerMemberID, ok := caller.MemberID(r.Context())
		if !ok {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		req.MemberID = callerMemberID
	}

	schema, err := h.service.CreateSchema(&req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), nil, string(middleware.AuditStatusFailure))

		status := http.StatusBadRequest
		if errors.Is(err, ErrSchemaAlreadyExists) {
			status = http.StatusConflict
		}
		utils.RespondWithError(w, status, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schema.SchemaID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, schema)
}

// UpdateSchema handles PUT /api/v1/schemas/{schemaId}
func (h *Handler) UpdateSchema(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")

	// Get existing schema to check ownership
	existingSchema, err := h.service.GetSchema(schemaId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeSchemaWriteAll, existingSchema.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
		return
	}

	var req UpdateSchemaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	schema, err := h.service.UpdateSchema(schemaId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &existingSchema.SchemaID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schema.SchemaID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, schema)
}

// Schema policy metadata handlers

// authorizeSchemaAccess checks that the caller owns the schema, unless it
// holds allScope, which covers every organization unit. It writes the error
// response and returns false when access is denied.
func (h *Handler) authorizeSchemaAccess(w http.ResponseWriter, r *http.Request, schemaId string, allScope string) bool {
	schema, err := h.service.GetSchema(schemaId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return false
	}

	if !caller.CanAccess(r.Context(), allScope, schema.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return false
	}

	return true
}

// ListSchemaPolicyMetadata handles GET /api/v1/schemas/{schemaId}/policy-metadata
func (h *Handler) ListSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	if !h.authorizeSchemaAccess(w, r, schemaId, caller.ScopeSchemaReadAll) {
		return
	}

	list, err := h.service.ListPolicyMetadata(schemaId)
	if err != nil {
		respondWithPolicyMetadataError(w, err)
		return
	}

	response := kernel.CollectionResponse{
		Items: list.Records,
		Count: len(list.Records),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// PatchSchemaPolicyMetadata handles PATCH /api/v1/schemas/{schemaId}/policy-metadata/{id}
func (h *Handler) PatchSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	if !h.authorizeSchemaAccess(w, r, schemaId, caller.ScopeSchemaWriteAll) {
		return
	}

	var req policy.PolicyMetadataPatchRequest
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

	record, err := h.service.PatchPolicyMetadata(schemaId, id, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, record)
}

// DeleteSchemaPolicyMetadata handles DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}
func (h *Handler) DeleteSchemaPolicyMetadata(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	if !h.authorizeSchemaAccess(w, r, schemaId, caller.ScopeSchemaWriteAll) {
		return
	}

	if err := h.service.DeletePolicyMetadata(schemaId, id); err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusSuccess))

	w.WriteHeader(http.StatusNoContent)
}

// RevokeSchemaPolicyAllowListEntry handles DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}/allowlist/{applicationId}
func (h *Handler) RevokeSchemaPolicyAllowListEntry(w http.ResponseWriter, r *http.Request) {
	schemaId := r.PathValue("schemaId")
	id := r.PathValue("id")
	applicationId := r.PathValue("applicationId")
	if !h.authorizeSchemaAccess(w, r, schemaId, caller.ScopeSchemaWriteAll) {
		return
	}

	if err := h.service.RevokeAllowListEntry(schemaId, id, applicationId); err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusFailure))

		respondWithPolicyMetadataError(w, err)
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeSchemas), &schemaId, string(middleware.AuditStatusSuccess))

	w.WriteHeader(http.StatusNoContent)
}

// respondWithPolicyMetadataError maps policy metadata errors onto a response.
// Client errors reported by the PDP are passed through, while PDP failures and
// unreachable PDPs are reported as a bad gateway.
func respondWithPolicyMetadataError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrPolicyMetadataNotFound) || errors.Is(err, ErrAllowListEntryNotFound) {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	var pdpErr *policy.Error
	if errors.As(err, &pdpErr) {
		switch pdpErr.StatusCode {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
			utils.RespondWithError(w, pdpErr.StatusCode, pdpErr.Message)
			return
		}
	}

	slog.Error("Policy metadata request to PDP failed", "error", err)
	utils.RespondWithError(w, http.StatusBadGateway, "Policy decision point request failed")
}
