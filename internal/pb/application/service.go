package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pb/idp"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"gorm.io/gorm"
)

// Service handles application-related operations
type Service struct {
	db            *gorm.DB
	policyService *policy.Client
	idp           idp.ApplicationManager
}

// NewService creates a new application service
func NewService(db *gorm.DB, pdpService *policy.Client, idp idp.ApplicationManager) *Service {
	return &Service{db: db, policyService: pdpService, idp: idp}
}

// CreateApplication creates a new application. Unless the caller supplies its
// IDP client, the application's OAuth2 client is created in the IDP, in the
// owning member's organization unit, and its client secret is returned once in
// the response. If req.IdpApplicationID and req.IdpClientID are both set, the
// caller has already provisioned the OAuth2 client directly in the IDP - IDP
// creation is skipped and those values are used as-is.
func (s *Service) CreateApplication(ctx context.Context, req *CreateApplicationRequest) (*ApplicationResponse, error) {
	externallyProvisioned := req.IdpApplicationID != nil || req.IdpClientID != nil
	if externallyProvisioned && (req.IdpApplicationID == nil || req.IdpClientID == nil) {
		return nil, fmt.Errorf("idpApplicationId and idpClientId must both be provided together, or both omitted")
	}
	if externallyProvisioned && (*req.IdpApplicationID == "" || *req.IdpClientID == "") {
		return nil, fmt.Errorf("idpApplicationId and idpClientId must not be empty")
	}

	var idpApplicationID *string
	var idpClientID *string
	// Only returned in this response, never stored.
	var idpClientSecret *string

	if externallyProvisioned {
		idpApplicationID = req.IdpApplicationID
		idpClientID = req.IdpClientID
	} else {
		// Step 1: Create Application in the IDP
		description := ""
		if req.ApplicationDescription != nil {
			description = *req.ApplicationDescription
		}

		// The IdP application lives in its member's organization unit.
		var owner member.Member
		if err := s.db.WithContext(ctx).Select("ou_id").First(&owner, "member_id = ?", req.MemberID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("member not found")
			}
			return nil, fmt.Errorf("failed to find member: %w", err)
		}

		applicationInstance := &idp.Application{
			Name:        req.ApplicationName,
			Description: description,
			TemplateId:  TemplateIDM2M,
			OuID:        owner.OuID,
		}
		created, err := s.idp.CreateApplication(ctx, applicationInstance)
		if err != nil {
			return nil, fmt.Errorf("failed to create application: %w", err)
		}
		idpApplicationID = &created.Id
		idpClientID = &created.ClientId
		if created.ClientSecret != "" {
			idpClientSecret = &created.ClientSecret
		}
	}

	// Step 2: Create application in database
	application := Application{
		ApplicationID:          uuid.New().String(),
		ApplicationName:        req.ApplicationName,
		ApplicationDescription: req.ApplicationDescription,
		SelectedFields:         SelectedFieldRecords(req.SelectedFields),
		IdpApplicationID:       idpApplicationID,
		IdpClientID:            idpClientID,
		MemberID:               req.MemberID,
		Version:                string(kernel.ActiveVersion),
	}

	if err := s.db.WithContext(ctx).Create(&application).Error; err != nil {
		// Compensation: delete the application from the IDP, but only if we're
		// the ones who created it there - an externally provisioned client
		// isn't ours to delete.
		if !externallyProvisioned {
			if deleteErr := s.idp.DeleteApplication(ctx, *idpApplicationID); deleteErr != nil {
				// Log the compensation failure - this needs monitoring
				slog.ErrorContext(ctx, "Failed to compensate application creation",
					"applicationID", application.ApplicationID,
					"originalError", err,
					"compensationError", deleteErr)
				// Return both errors for visibility
				return nil, fmt.Errorf("failed to create application: %w, and failed to compensate: %w", err, deleteErr)
			}
			slog.InfoContext(ctx, "Successfully compensated application creation", "applicationID", application.ApplicationID)
		}
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	// Step 3: Grant the application access to its fields in the PDP (Saga Pattern)
	// Grants are keyed by the IdP OIDC client_id, not the portal's random
	// ApplicationID UUID. This is the identifier the Orchestration Engine extracts
	// from the IdP-issued token (via the client_id fallback) when asking the PDP for
	// a decision, so both sides must agree on it. See issue #447.
	if application.IdpClientID == nil {
		return nil, fmt.Errorf("cannot grant access: application IdpClientID is nil")
	}
	policyReq := policy.AccessGrantCreateRequest{
		ApplicationID: *application.IdpClientID,
		Records:       application.SelectedFields,
		GrantDuration: policy.GrantDurationTypeOneMonth, // Default duration
	}

	_, err := s.policyService.CreateAccessGrants(policyReq)
	if err != nil {
		// Compensation: Attempt both cleanup operations regardless of individual failures
		// This ensures we don't leave orphaned resources in either system
		var dbDeleteErr, idpDeleteErr error

		// Attempt to delete from database
		dbDeleteErr = s.db.Delete(&application).Error
		if dbDeleteErr != nil {
			slog.ErrorContext(ctx, "Failed to delete application from database during compensation",
				"applicationID", application.ApplicationID,
				"originalError", err,
				"compensationError", dbDeleteErr)
		}

		// Attempt to delete from IDP regardless of database deletion result -
		// but only if we created it there; an externally provisioned client
		// isn't ours to delete.
		if !externallyProvisioned {
			idpDeleteErr = s.idp.DeleteApplication(ctx, *application.IdpApplicationID)
			if idpDeleteErr != nil {
				slog.ErrorContext(ctx, "Failed to delete application from IDP during compensation",
					"applicationID", application.ApplicationID,
					"idpApplicationID", *application.IdpApplicationID,
					"originalError", err,
					"compensationError", idpDeleteErr)
			}
		}

		// Determine the appropriate error response based on what failed
		if dbDeleteErr != nil && idpDeleteErr != nil {
			return nil, fmt.Errorf("failed to update allow list: %w, and failed to compensate (DB error: %v, IDP error: %v)", err, dbDeleteErr, idpDeleteErr)
		} else if dbDeleteErr != nil {
			return nil, fmt.Errorf("failed to update allow list: %w, and failed to compensate database deletion: %w", err, dbDeleteErr)
		} else if idpDeleteErr != nil {
			return nil, fmt.Errorf("failed to update allow list: %w, and failed to compensate IDP deletion: %w", err, idpDeleteErr)
		}

		slog.InfoContext(ctx, "Successfully compensated application creation", "applicationID", application.ApplicationID)
		return nil, fmt.Errorf("failed to update allow list: %w", err)
	}

	response := &ApplicationResponse{
		ApplicationID:          application.ApplicationID,
		ApplicationName:        application.ApplicationName,
		ApplicationDescription: application.ApplicationDescription,
		SelectedFields:         application.SelectedFields,
		MemberID:               application.MemberID,
		Version:                application.Version,
		IdpApplicationID:       application.IdpApplicationID,
		IdpClientID:            application.IdpClientID,
		IdpClientSecret:        idpClientSecret,
		CreatedAt:              application.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              application.UpdatedAt.Format(time.RFC3339),
	}

	return response, nil
}

// UpdateApplication updates an existing application
func (s *Service) UpdateApplication(ctx context.Context, applicationID string, req *UpdateApplicationRequest) (*ApplicationResponse, error) {
	var application Application
	err := s.db.WithContext(ctx).First(&application, "application_id = ?", applicationID).Error
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	// Note: SelectedFields updates are intentionally not supported for approved applications
	// to maintain data integrity and audit trail. Field changes require resubmission process.
	if req.ApplicationName != nil {
		application.ApplicationName = *req.ApplicationName
	}
	if req.ApplicationDescription != nil {
		application.ApplicationDescription = req.ApplicationDescription
	}
	if req.Version != nil {
		application.Version = *req.Version
	}

	if err := s.db.WithContext(ctx).Save(&application).Error; err != nil {
		return nil, err
	}

	response := &ApplicationResponse{
		ApplicationID:          application.ApplicationID,
		ApplicationName:        application.ApplicationName,
		ApplicationDescription: application.ApplicationDescription,
		SelectedFields:         application.SelectedFields,
		MemberID:               application.MemberID,
		Version:                application.Version,
		IdpApplicationID:       application.IdpApplicationID,
		IdpClientID:            application.IdpClientID,
		CreatedAt:              application.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              application.UpdatedAt.Format(time.RFC3339),
	}
	if application.ApplicationDescription != nil && *application.ApplicationDescription != "" {
		response.ApplicationDescription = application.ApplicationDescription
	}

	return response, nil
}

// UpdateApplicationPolicy replaces an existing application's allow-list in the PDP and
// persists the resulting selected fields on the application record.
func (s *Service) UpdateApplicationPolicy(ctx context.Context, applicationID string, req *UpdateApplicationPolicyRequest) (*ApplicationResponse, error) {
	var application Application
	if err := s.db.WithContext(ctx).First(&application, "application_id = ?", applicationID).Error; err != nil {
		return nil, err
	}

	if application.IdpClientID == nil {
		return nil, fmt.Errorf("cannot update policy: application IdpClientID is nil")
	}

	grantDuration := policy.GrantDurationTypeOneMonth
	if req.GrantDuration != nil {
		grantDuration = *req.GrantDuration
	}

	policyReq := policy.AccessGrantCreateRequest{
		ApplicationID: *application.IdpClientID,
		Records:       req.SelectedFields,
		GrantDuration: grantDuration,
	}

	if _, err := s.policyService.CreateAccessGrants(policyReq); err != nil {
		return nil, fmt.Errorf("failed to update allow list: %w", err)
	}

	application.SelectedFields = SelectedFieldRecords(req.SelectedFields)
	if err := s.db.WithContext(ctx).Save(&application).Error; err != nil {
		return nil, fmt.Errorf("failed to persist updated selected fields: %w", err)
	}

	response := &ApplicationResponse{
		ApplicationID:          application.ApplicationID,
		ApplicationName:        application.ApplicationName,
		ApplicationDescription: application.ApplicationDescription,
		SelectedFields:         application.SelectedFields,
		MemberID:               application.MemberID,
		Version:                application.Version,
		IdpApplicationID:       application.IdpApplicationID,
		IdpClientID:            application.IdpClientID,
		CreatedAt:              application.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              application.UpdatedAt.Format(time.RFC3339),
	}

	return response, nil
}

// GetApplication retrieves an application by ID
func (s *Service) GetApplication(ctx context.Context, applicationID string) (*ApplicationResponse, error) {
	var application Application
	err := s.db.WithContext(ctx).Preload("Member").First(&application, "application_id = ?", applicationID).Error
	if err != nil {
		return nil, err
	}

	response := &ApplicationResponse{
		ApplicationID:          application.ApplicationID,
		ApplicationName:        application.ApplicationName,
		ApplicationDescription: application.ApplicationDescription,
		SelectedFields:         application.SelectedFields,
		MemberID:               application.MemberID,
		Version:                application.Version,
		IdpApplicationID:       application.IdpApplicationID,
		IdpClientID:            application.IdpClientID,
		CreatedAt:              application.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              application.UpdatedAt.Format(time.RFC3339),
	}
	if application.ApplicationDescription != nil && *application.ApplicationDescription != "" {
		response.ApplicationDescription = application.ApplicationDescription
	}

	return response, nil
}

// GetApplications retrieves all applications and filters by member ID if provided
func (s *Service) GetApplications(ctx context.Context, MemberID *string) ([]ApplicationResponse, error) {
	var applications []Application
	query := s.db.WithContext(ctx).Preload("Member")
	if MemberID != nil && *MemberID != "" {
		query = query.Where("member_id = ?", *MemberID)
	}

	// Order by created_at descending
	query = query.Order("created_at DESC")

	err := query.Find(&applications).Error
	if err != nil {
		return nil, err
	}

	// Pre-allocate slice with known capacity for better performance
	responses := make([]ApplicationResponse, 0, len(applications))
	for _, application := range applications {
		resp := ApplicationResponse{
			ApplicationID:    application.ApplicationID,
			ApplicationName:  application.ApplicationName,
			SelectedFields:   application.SelectedFields,
			MemberID:         application.MemberID,
			IdpApplicationID: application.IdpApplicationID,
			IdpClientID:      application.IdpClientID,
			Version:          application.Version,
			CreatedAt:        application.CreatedAt.Format(time.RFC3339),
			UpdatedAt:        application.UpdatedAt.Format(time.RFC3339),
		}
		if application.ApplicationDescription != nil && *application.ApplicationDescription != "" {
			resp.ApplicationDescription = application.ApplicationDescription
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

// CreateApplicationSubmission creates a new application submission
func (s *Service) CreateApplicationSubmission(ctx context.Context, req *CreateApplicationSubmissionRequest) (*ApplicationSubmissionResponse, error) {
	// Validate previous application ID if provided
	if req.PreviousApplicationID != nil {
		var prevApp Application
		err := s.db.WithContext(ctx).First(&prevApp, "application_id = ?", *req.PreviousApplicationID).Error
		if err != nil {
			return nil, err
		}
	}

	// Validate member ID
	var owner member.Member
	err := s.db.WithContext(ctx).First(&owner, "member_id = ?", req.MemberID).Error
	if err != nil {
		return nil, err
	}

	// Create application submission
	submission := ApplicationSubmission{
		SubmissionID:           uuid.New().String(),
		PreviousApplicationID:  req.PreviousApplicationID,
		ApplicationName:        req.ApplicationName,
		ApplicationDescription: req.ApplicationDescription,
		SelectedFields:         SelectedFieldRecords(req.SelectedFields),
		Status:                 string(kernel.StatusPending),
		MemberID:               req.MemberID,
	}
	if err := s.db.WithContext(ctx).Create(&submission).Error; err != nil {
		return nil, err
	}

	response := &ApplicationSubmissionResponse{
		SubmissionID:           submission.SubmissionID,
		PreviousApplicationID:  submission.PreviousApplicationID,
		ApplicationName:        submission.ApplicationName,
		ApplicationDescription: submission.ApplicationDescription,
		SelectedFields:         submission.SelectedFields,
		Status:                 submission.Status,
		MemberID:               submission.MemberID,
		CreatedAt:              submission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              submission.UpdatedAt.Format(time.RFC3339),
	}

	return response, nil
}

// UpdateApplicationSubmission updates an existing application submission
func (s *Service) UpdateApplicationSubmission(ctx context.Context, submissionID string, req *UpdateApplicationSubmissionRequest) (*ApplicationSubmissionResponse, error) {
	var submission ApplicationSubmission

	// Find the submission
	if err := s.db.WithContext(ctx).First(&submission, "submission_id = ?", submissionID).Error; err != nil {
		return nil, fmt.Errorf("application submission not found: %w", err)
	}

	// Validate PreviousApplicationID first before making any updates
	if req.PreviousApplicationID != nil {
		// Validate previous application ID
		var prevApp Application
		if err := s.db.WithContext(ctx).First(&prevApp, "application_id = ?", *req.PreviousApplicationID).Error; err != nil {
			return nil, fmt.Errorf("previous application not found: %w", err)
		}
	}

	// Update fields if provided
	if req.ApplicationName != nil {
		submission.ApplicationName = *req.ApplicationName
	}
	if req.ApplicationDescription != nil {
		submission.ApplicationDescription = req.ApplicationDescription
	}

	if req.SelectedFields != nil && len(*req.SelectedFields) > 0 {
		submission.SelectedFields = *req.SelectedFields
	}

	if req.PreviousApplicationID != nil {
		submission.PreviousApplicationID = req.PreviousApplicationID
	}

	var shouldCreateApplication bool
	if req.Status != nil {
		submission.Status = *req.Status
		// Mark that we need to create an application after saving
		if *req.Status == string(kernel.StatusApproved) {
			shouldCreateApplication = true
		}
	}

	if req.Review != nil {
		submission.Review = req.Review
	}

	// Save the updated submission
	if err := s.db.WithContext(ctx).Save(&submission).Error; err != nil {
		return nil, fmt.Errorf("failed to update application submission: %w", err)
	}

	// Create application outside of transaction if approval was successful
	if shouldCreateApplication {
		var createApplicationRequest CreateApplicationRequest
		createApplicationRequest.ApplicationName = submission.ApplicationName
		createApplicationRequest.ApplicationDescription = submission.ApplicationDescription
		createApplicationRequest.SelectedFields = SelectedFieldRecords(submission.SelectedFields)
		createApplicationRequest.MemberID = submission.MemberID

		_, err := s.CreateApplication(ctx, &createApplicationRequest)
		if err != nil {
			// Compensation: Update submission status back to pending
			submission.Status = string(kernel.StatusPending)
			if updateErr := s.db.WithContext(ctx).Save(&submission).Error; updateErr != nil {
				slog.ErrorContext(ctx, "Failed to compensate submission status after application creation failure",
					"submissionID", submission.SubmissionID,
					"originalError", err,
					"compensationError", updateErr)
				return nil, fmt.Errorf("failed to create application from approved submission: %w, and failed to compensate submission status: %w", err, updateErr)
			}
			slog.InfoContext(ctx, "Successfully compensated submission status after application creation failure", "submissionID", submission.SubmissionID)
			return nil, fmt.Errorf("failed to create application from approved submission: %w", err)
		}
	}

	response := &ApplicationSubmissionResponse{
		SubmissionID:           submission.SubmissionID,
		PreviousApplicationID:  submission.PreviousApplicationID,
		ApplicationName:        submission.ApplicationName,
		ApplicationDescription: submission.ApplicationDescription,
		SelectedFields:         submission.SelectedFields,
		Status:                 submission.Status,
		MemberID:               submission.MemberID,
		CreatedAt:              submission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              submission.UpdatedAt.Format(time.RFC3339),
		Review:                 submission.Review,
	}

	return response, nil
}

// GetApplicationSubmission retrieves an application submission by ID
func (s *Service) GetApplicationSubmission(ctx context.Context, submissionID string) (*ApplicationSubmissionResponse, error) {
	var submission ApplicationSubmission
	err := s.db.WithContext(ctx).Preload("Member").Preload("PreviousApplication").First(&submission, "submission_id = ?", submissionID).Error
	if err != nil {
		return nil, err
	}

	response := &ApplicationSubmissionResponse{
		SubmissionID:           submission.SubmissionID,
		PreviousApplicationID:  submission.PreviousApplicationID,
		ApplicationName:        submission.ApplicationName,
		ApplicationDescription: submission.ApplicationDescription,
		SelectedFields:         submission.SelectedFields,
		Status:                 submission.Status,
		MemberID:               submission.MemberID,
		CreatedAt:              submission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:              submission.UpdatedAt.Format(time.RFC3339),
		Review:                 submission.Review,
	}

	return response, nil
}

// GetApplicationSubmissions retrieves all application submissions and filters by member ID if provided
func (s *Service) GetApplicationSubmissions(ctx context.Context, MemberID *string, statusFilter *[]string) ([]ApplicationSubmissionResponse, error) {
	var submissions []ApplicationSubmission
	query := s.db.WithContext(ctx).Preload("Member").Preload("PreviousApplication")
	if MemberID != nil && *MemberID != "" {
		query = query.Where("member_id = ?", *MemberID)
	}
	if statusFilter != nil && len(*statusFilter) > 0 {
		query = query.Where("status IN ?", *statusFilter)
	}

	// Order by created_at descending
	query = query.Order("created_at DESC")

	err := query.Find(&submissions).Error
	if err != nil {
		return nil, err
	}

	var responses []ApplicationSubmissionResponse
	for _, submission := range submissions {
		responses = append(responses, ApplicationSubmissionResponse{
			SubmissionID:           submission.SubmissionID,
			PreviousApplicationID:  submission.PreviousApplicationID,
			ApplicationName:        submission.ApplicationName,
			ApplicationDescription: submission.ApplicationDescription,
			SelectedFields:         submission.SelectedFields,
			Status:                 submission.Status,
			MemberID:               submission.MemberID,
			CreatedAt:              submission.CreatedAt.Format(time.RFC3339),
			UpdatedAt:              submission.UpdatedAt.Format(time.RFC3339),
			Review:                 submission.Review,
		})
	}

	return responses, nil
}
