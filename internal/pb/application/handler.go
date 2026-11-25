// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package application

import (
	"encoding/json"
	"net/http"

	"github.com/openndx/openndx-core/internal/pb/caller"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/utils"
)

// Handler handles the application and application submission HTTP API
type Handler struct {
	service *Service
}

// NewHandler creates a new application Handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Application submission handlers

// GetAllApplicationSubmissions handles GET /api/v1/application-submissions
func (h *Handler) GetAllApplicationSubmissions(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")
	statusFilter := r.URL.Query()["status"]

	// Unless the caller reads across organization units, limit it to its own submissions
	if !caller.HasScope(r.Context(), caller.ScopeApplicationSubmissionReadAll) {
		callerMemberID, ok := caller.MemberID(r.Context())
		if !ok {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		memberId = callerMemberID
	}

	submissions, err := h.service.GetApplicationSubmissions(r.Context(), &memberId, &statusFilter)
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

// GetApplicationSubmission handles GET /api/v1/application-submissions/{submissionId}
func (h *Handler) GetApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	submission, err := h.service.GetApplicationSubmission(r.Context(), submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeApplicationSubmissionReadAll, submission.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// CreateApplicationSubmission handles POST /api/v1/application-submissions
func (h *Handler) CreateApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	var req CreateApplicationSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Unless the caller writes across organization units, ensure they can only create submissions for themselves
	if !caller.HasScope(r.Context(), caller.ScopeApplicationSubmissionWriteAll) {
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

	submission, err := h.service.CreateApplicationSubmission(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplicationSubmissions), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplicationSubmissions), &submission.SubmissionID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, submission)
}

// UpdateApplicationSubmission handles PUT /api/v1/application-submissions/{submissionId}
func (h *Handler) UpdateApplicationSubmission(w http.ResponseWriter, r *http.Request) {
	submissionId := r.PathValue("submissionId")

	// Get existing submission to check ownership
	existingSubmission, err := h.service.GetApplicationSubmission(r.Context(), submissionId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, "Application submission not found")
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeApplicationSubmissionWriteAll, existingSubmission.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return
	}

	var req UpdateApplicationSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	submission, err := h.service.UpdateApplicationSubmission(r.Context(), submissionId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplicationSubmissions), &existingSubmission.SubmissionID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplicationSubmissions), &submission.SubmissionID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, submission)
}

// Application handlers

// GetAllApplications handles GET /api/v1/applications
func (h *Handler) GetAllApplications(w http.ResponseWriter, r *http.Request) {
	memberId := r.URL.Query().Get("memberId")

	// Unless the caller reads across organization units, limit it to its own applications
	if !caller.HasScope(r.Context(), caller.ScopeApplicationReadAll) {
		callerMemberID, ok := caller.MemberID(r.Context())
		if !ok {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		memberId = callerMemberID
	}

	applications, err := h.service.GetApplications(r.Context(), &memberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := kernel.CollectionResponse{
		Items: applications,
		Count: len(applications),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}

// GetApplication handles GET /api/v1/applications/{applicationId}
func (h *Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	application, err := h.service.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeApplicationReadAll, application.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, application)
}

// CreateApplication handles POST /api/v1/applications
func (h *Handler) CreateApplication(w http.ResponseWriter, r *http.Request) {
	var req CreateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Unless the caller writes across organization units, it creates applications for its own member
	if !caller.HasScope(r.Context(), caller.ScopeApplicationWriteAll) {
		callerMemberID, ok := caller.MemberID(r.Context())
		if !ok {
			utils.RespondWithError(w, http.StatusForbidden, "User member record not found")
			return
		}
		req.MemberID = callerMemberID
	}

	application, err := h.service.CreateApplication(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &application.ApplicationID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, application)
}

// UpdateApplication handles PUT /api/v1/applications/{applicationId}
func (h *Handler) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	// Get existing application to check ownership
	existingApplication, err := h.service.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeApplicationWriteAll, existingApplication.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
		return
	}

	var req UpdateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	application, err := h.service.UpdateApplication(r.Context(), applicationId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &existingApplication.ApplicationID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &application.ApplicationID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, application)
}

// UpdateApplicationPolicy handles PUT /api/v1/applications/{applicationId}/policy
func (h *Handler) UpdateApplicationPolicy(w http.ResponseWriter, r *http.Request) {
	applicationId := r.PathValue("applicationId")

	// Get existing application to check ownership
	existingApplication, err := h.service.GetApplication(r.Context(), applicationId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	if !caller.CanAccess(r.Context(), caller.ScopeApplicationWriteAll, existingApplication.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
		return
	}

	var req UpdateApplicationPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	application, err := h.service.UpdateApplicationPolicy(r.Context(), applicationId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &existingApplication.ApplicationID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeApplications), &application.ApplicationID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, application)
}
