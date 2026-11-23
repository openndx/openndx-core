// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package member

import (
	"encoding/json"
	"net/http"

	"github.com/openndx/openndx-core/internal/pb/caller"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/middleware"
	"github.com/openndx/openndx-core/internal/utils"
)

// Handler handles the member HTTP API
type Handler struct {
	service *Service
}

// NewHandler creates a new member Handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateMember handles POST /api/v1/members
func (h *Handler) CreateMember(w http.ResponseWriter, r *http.Request) {
	var req CreateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	member, err := h.service.CreateMember(r.Context(), &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeMembers), nil, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeMembers), &member.MemberID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusCreated, member)
}

// UpdateMember handles PUT /api/v1/members/{memberId}
func (h *Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	memberId := r.PathValue("memberId")

	// Get the existing member to check ownership
	existingMember, err := h.service.GetMember(r.Context(), memberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// Callers writing across organization units can update any member, others only their own
	if !caller.CanAccess(r.Context(), caller.ScopeMemberWriteAll, existingMember.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to update this resource")
		return
	}

	var req UpdateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Pass request context to service for proper context propagation
	member, err := h.service.UpdateMember(r.Context(), memberId, &req)
	if err != nil {
		// Log audit event for failure
		middleware.LogAuditEvent(r, string(middleware.ResourceTypeMembers), &existingMember.MemberID, string(middleware.AuditStatusFailure))

		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log audit event
	middleware.LogAuditEvent(r, string(middleware.ResourceTypeMembers), &member.MemberID, string(middleware.AuditStatusSuccess))

	utils.RespondWithSuccess(w, http.StatusOK, member)
}

// GetMember handles GET /api/v1/members/{memberId}
func (h *Handler) GetMember(w http.ResponseWriter, r *http.Request) {
	memberId := r.PathValue("memberId")

	// Get the member from database
	// Pass request context to service for proper context propagation
	member, err := h.service.GetMember(r.Context(), memberId)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// Callers reading across organization units can access any member, others only their own
	if !caller.CanAccess(r.Context(), caller.ScopeMemberReadAll, member.MemberID) {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied to this resource")
		return
	}

	utils.RespondWithSuccess(w, http.StatusOK, member)
}

// GetAllMembers handles GET /api/v1/members
func (h *Handler) GetAllMembers(w http.ResponseWriter, r *http.Request) {
	ouID := r.URL.Query().Get("ouId")

	// Callers reading across organization units can filter by organization unit
	// or see every member, others only their own organization unit's
	if !caller.HasScope(r.Context(), caller.ScopeMemberReadAll) {
		ouID = caller.OUID(r.Context())
		if ouID == "" {
			utils.RespondWithError(w, http.StatusForbidden, "User has no organization unit")
			return
		}
	}

	// Pass request context to service for proper context propagation
	members, err := h.service.GetAllMembers(r.Context(), &ouID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := kernel.CollectionResponse{
		Items: members,
		Count: len(members),
	}
	utils.RespondWithSuccess(w, http.StatusOK, response)
}
