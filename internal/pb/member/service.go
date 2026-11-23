// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package member

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrMemberAlreadyExists is returned when a member is already linked to the
// requested organization unit
var ErrMemberAlreadyExists = errors.New("a member is already linked to this organization unit")

// Service handles Member-related operations
type Service struct {
	db *gorm.DB
}

// NewService creates a new Member service
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreateMember creates a new Member organization linked to an existing
// ThunderID organization unit. The OU and its users are managed in ThunderID;
// nothing is provisioned in the IDP here.
func (s *Service) CreateMember(ctx context.Context, req *CreateMemberRequest) (*MemberResponse, error) {
	if req.OuID == "" {
		return nil, fmt.Errorf("ouId must not be empty")
	}

	member := Member{
		MemberID:    uuid.New().String(),
		Name:        req.Name,
		Email:       req.Email,
		PhoneNumber: req.PhoneNumber,
		OuID:        req.OuID,
	}
	if err := s.db.Create(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrMemberAlreadyExists
		}
		return nil, fmt.Errorf("failed to create member in database: %w", err)
	}

	slog.InfoContext(ctx, "Created member successfully", "memberID", member.MemberID, "ouID", member.OuID)
	return s.buildMemberResponse(&member), nil
}

// UpdateMember updates an existing Member. The OU link cannot be changed.
func (s *Service) UpdateMember(ctx context.Context, memberID string, req *UpdateMemberRequest) (*MemberResponse, error) {
	var member Member
	err := s.db.First(&member, "member_id = ?", memberID).Error
	if err != nil {
		return nil, fmt.Errorf("member not found: %w", err)
	}

	if req.Name != nil {
		member.Name = *req.Name
	}
	if req.Email != nil {
		member.Email = *req.Email
	}
	if req.PhoneNumber != nil {
		member.PhoneNumber = *req.PhoneNumber
	}

	if err := s.db.Save(&member).Error; err != nil {
		return nil, fmt.Errorf("failed to update member in database: %w", err)
	}

	slog.InfoContext(ctx, "Updated member successfully", "memberID", memberID)
	return s.buildMemberResponse(&member), nil
}

// GetMember retrieves a Member by ID
func (s *Service) GetMember(ctx context.Context, memberID string) (*MemberResponse, error) {
	var member Member
	err := s.db.Where("member_id = ?", memberID).First(&member).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch member: %w", err)
	}

	return s.buildMemberResponse(&member), nil
}

// GetAllMembers retrieves all members, optionally filtered by ouId
func (s *Service) GetAllMembers(ctx context.Context, ouID *string) ([]MemberResponse, error) {
	// Handle filtered query
	if ouID != nil && *ouID != "" {
		return s.getMembersByOuID(ctx, *ouID)
	}

	// Handle all members query
	return s.getAllMembers(ctx)
}

// getMembersByOuID retrieves the member linked to the given organization
// unit, or an empty list if there is none
func (s *Service) getMembersByOuID(ctx context.Context, ouID string) ([]MemberResponse, error) {
	var member Member
	err := s.db.Where("ou_id = ?", ouID).First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []MemberResponse{}, nil
		}
		return nil, fmt.Errorf("failed to fetch member: %w", err)
	}

	return []MemberResponse{*s.buildMemberResponse(&member)}, nil
}

// getAllMembers retrieves all members
func (s *Service) getAllMembers(ctx context.Context) ([]MemberResponse, error) {
	var members []Member
	err := s.db.Find(&members).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch members: %w", err)
	}

	response := make([]MemberResponse, len(members))
	for i, member := range members {
		response[i] = *s.buildMemberResponse(&member)
	}

	return response, nil
}

// MemberIDByOuID returns the ID of the member linked to the given
// organization unit, or "" when none is. It is the caller.MemberLookup PB
// uses to find a caller's member.
func (s *Service) MemberIDByOuID(ctx context.Context, ouID string) (string, error) {
	if ouID == "" {
		return "", nil
	}

	members, err := s.getMembersByOuID(ctx, ouID)
	if err != nil || len(members) == 0 {
		return "", err
	}
	return members[0].MemberID, nil
}

// buildMemberResponse converts a Member model to MemberResponse
func (s *Service) buildMemberResponse(member *Member) *MemberResponse {
	return &MemberResponse{
		MemberID:    member.MemberID,
		Name:        member.Name,
		Email:       member.Email,
		PhoneNumber: member.PhoneNumber,
		OuID:        member.OuID,
		CreatedAt:   member.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   member.UpdatedAt.Format(time.RFC3339),
	}
}
