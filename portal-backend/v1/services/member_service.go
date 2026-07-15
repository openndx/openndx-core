package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gov-dx-sandbox/portal-backend/idp"
	"github.com/gov-dx-sandbox/portal-backend/v1/models"
	"gorm.io/gorm"
)

// MemberService handles Member-related operations
type MemberService struct {
	db  *gorm.DB
	idp idp.IdentityProviderAPI
}

// NewMemberService creates a new Member service
func NewMemberService(db *gorm.DB, idp idp.IdentityProviderAPI) *MemberService {
	return &MemberService{db: db, idp: idp}
}

// CreateMember creates a new Member and automatically adds them to the "OpenDIF_Members" group in the IDP.
// This ensures all newly created members are automatically assigned to the member group.
func (s *MemberService) CreateMember(ctx context.Context, req *models.CreateMemberRequest) (*models.MemberResponse, error) {
	// Create user in the IDP
	userInstance := &idp.User{
		Email:       req.Email,
		FirstName:   req.Name,
		LastName:    "",
		PhoneNumber: req.PhoneNumber,
	}
	createdUser, err := s.idp.CreateUser(ctx, userInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create user in IDP: %w", err)
	}
	if createdUser.Email != userInstance.Email {
		deleteErr := (s.idp).DeleteUser(ctx, createdUser.Id)
		if deleteErr != nil {
			return nil, fmt.Errorf("IDP user email mismatch, and failed to rollback user creation in IDP: %w", deleteErr)
		}
		return nil, fmt.Errorf("IDP user email mismatch: expected %s, got %s", userInstance.Email, createdUser.Email)
	}
	slog.Info("Created user in IDP", "userID", createdUser.Id, "email", createdUser.Email)

	// Automatically add user to "OpenDIF_Members" group in the IDP
	// This is a core requirement: all new members must be assigned to the member group
	groupMember := &idp.GroupMember{
		Value:   createdUser.Id,
		Display: createdUser.Email,
	}
	groupId, err := s.idp.AddMemberToGroupByGroupName(ctx, string(models.UserGroupMember), groupMember)
	if err != nil {
		// Rollback: Delete the user we just created
		deleteErr := s.idp.DeleteUser(ctx, createdUser.Id)
		if deleteErr != nil {
			return nil, fmt.Errorf("failed to add user to group %s: %w (rollback also failed: %v)", models.UserGroupMember, err, deleteErr)
		}
		return nil, fmt.Errorf("failed to add user to group %s: %w", models.UserGroupMember, err)
	}
	slog.Info("Added user to group", "userID", createdUser.Id, "groupId", *groupId, "groupName", models.UserGroupMember)

	// Create Member in the database
	member := models.Member{
		MemberID:    "mem_" + uuid.New().String(),
		Name:        req.Name,
		Email:       req.Email,
		PhoneNumber: req.PhoneNumber,
		IdpUserID:   createdUser.Id,
	}
	if dbErr := s.db.Create(&member).Error; dbErr != nil {
		// Rollback: Remove user from group and delete user from IDP
		var rollbackErrs []error
		if removeErr := s.idp.RemoveMemberFromGroup(ctx, *groupId, createdUser.Id); removeErr != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback group removal: %w", removeErr))
		}
		if deleteErr := s.idp.DeleteUser(ctx, createdUser.Id); deleteErr != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("rollback user deletion: %w", deleteErr))
		}
		if len(rollbackErrs) > 0 {
			return nil, fmt.Errorf("failed to create member in database: %w, rollback errors: %v", dbErr, errors.Join(rollbackErrs...))
		}
		return nil, fmt.Errorf("failed to create member in database: %w", dbErr)
	}

	slog.Info("Created member successfully", "memberID", member.MemberID, "email", member.Email)
	return s.buildMemberResponse(&member), nil
}

// UpdateMember updates an existing Member
func (s *MemberService) UpdateMember(ctx context.Context, memberID string, req *models.UpdateMemberRequest) (*models.MemberResponse, error) {
	var member models.Member
	err := s.db.First(&member, "member_id = ?", memberID).Error
	if err != nil {
		return nil, fmt.Errorf("member not found: %w", err)
	}

	// Store original values for rollback if needed
	originalName := member.Name
	originalPhoneNumber := member.PhoneNumber

	// Check if we need to update the IDP user
	needsIdpUpdate := false

	// Update fields if provided
	if req.Name != nil {
		member.Name = *req.Name
		needsIdpUpdate = true
	}
	if req.PhoneNumber != nil {
		member.PhoneNumber = *req.PhoneNumber
		needsIdpUpdate = true
	}

	// Update user in IDP if necessary
	if needsIdpUpdate {
		userInstance := &idp.User{
			Email:       member.Email,
			FirstName:   member.Name,
			LastName:    "",
			PhoneNumber: member.PhoneNumber,
		}

		_, err := s.idp.UpdateUser(ctx, member.IdpUserID, userInstance)
		if err != nil {
			return nil, fmt.Errorf("failed to update user in IDP: %w", err)
		}

		slog.Info("Updated user in IDP", "userID", member.IdpUserID)
	}

	// Update member in database
	if err := s.db.Save(&member).Error; err != nil {
		// Rollback IDP user update if DB operation fails
		if needsIdpUpdate {
			rollbackUser := &idp.User{
				Email:       member.Email,
				FirstName:   originalName,
				LastName:    "",
				PhoneNumber: originalPhoneNumber,
			}
			_, rollbackErr := s.idp.UpdateUser(ctx, member.IdpUserID, rollbackUser)
			if rollbackErr != nil {
				return nil, fmt.Errorf("failed to update member in database and failed to rollback IDP update: %w", errors.Join(err, fmt.Errorf("failed to rollback IDP update: %w", rollbackErr)))
			}
			slog.Warn("Rolled back IDP user update due to database failure", "userID", member.IdpUserID)
		}
		return nil, fmt.Errorf("failed to update member in database: %w", err)
	}

	slog.Info("Updated member successfully", "memberID", memberID)
	return s.buildMemberResponse(&member), nil
}

// GetMember retrieves a Member by ID
func (s *MemberService) GetMember(ctx context.Context, memberID string) (*models.MemberResponse, error) {
	var member models.Member
	err := s.db.Where("member_id = ?", memberID).First(&member).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch member: %w", err)
	}

	return s.buildMemberResponse(&member), nil
}

// GetAllMembers retrieves all members, optionally filtered by idpUserId or email
func (s *MemberService) GetAllMembers(ctx context.Context, idpUserId *string, email *string) ([]models.MemberResponse, error) {
	// Handle filtered query
	if (idpUserId != nil && *idpUserId != "") || (email != nil && *email != "") {
		return s.getFilteredMembers(ctx, idpUserId, email)
	}

	// Handle all members query
	return s.getAllMembers(ctx)
}

// getFilteredMembers retrieves members filtered by idpUserId or email
func (s *MemberService) getFilteredMembers(ctx context.Context, idpUserId *string, email *string) ([]models.MemberResponse, error) {
	var member models.Member
	query := s.db
	if idpUserId != nil && *idpUserId != "" {
		query = query.Where("idp_user_id = ?", *idpUserId)
	}
	if email != nil && *email != "" {
		query = query.Where("email = ?", *email)
	}
	err := query.First(&member).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch member: %w", err)
	}

	return []models.MemberResponse{*s.buildMemberResponse(&member)}, nil
}

// getAllMembers retrieves all members
func (s *MemberService) getAllMembers(ctx context.Context) ([]models.MemberResponse, error) {
	var members []models.Member
	err := s.db.Find(&members).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch members: %w", err)
	}

	response := make([]models.MemberResponse, len(members))
	for i, member := range members {
		response[i] = *s.buildMemberResponse(&member)
	}

	return response, nil
}

// buildMemberResponse converts a Member model to MemberResponse
func (s *MemberService) buildMemberResponse(member *models.Member) *models.MemberResponse {
	return &models.MemberResponse{
		MemberID:    member.MemberID,
		IdpUserID:   member.IdpUserID,
		Name:        member.Name,
		Email:       member.Email,
		PhoneNumber: member.PhoneNumber,
		CreatedAt:   member.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   member.UpdatedAt.Format(time.RFC3339),
	}
}
