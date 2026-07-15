package services

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SchemaService handles schema-related operations
type SchemaService struct {
	db            *gorm.DB
	policyService *PDPService
}

// NewSchemaService creates a new schema service
func NewSchemaService(db *gorm.DB, policyService *PDPService) *SchemaService {
	return &SchemaService{db: db, policyService: policyService}
}

// CreateSchema creates a new schema
func (s *SchemaService) CreateSchema(req *models.CreateSchemaRequest) (*models.SchemaResponse, error) {
	schema := models.Schema{
		SchemaID:   "sch_" + uuid.New().String(),
		SchemaName: req.SchemaName,
		SDL:        req.SDL,
		Endpoint:   req.Endpoint,
		MemberID:   req.MemberID,
		Version:    string(models.ActiveVersion),
	}
	if req.SchemaDescription != nil {
		schema.SchemaDescription = req.SchemaDescription
	}

	// Step 1: Create schema in database first
	if err := s.db.Create(&schema).Error; err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Step 2: Create policy metadata in PDP (Saga Pattern)
	_, err := s.policyService.CreatePolicyMetadata(schema.SchemaID, schema.SDL)
	if err != nil {
		// Compensation: Delete the schema we just created
		if deleteErr := s.db.Delete(&schema).Error; deleteErr != nil {
			// Log the compensation failure - this needs monitoring
			slog.Error("Failed to compensate schema creation",
				"schemaID", schema.SchemaID,
				"originalError", err,
				"compensationError", deleteErr)
			// Return both errors for visibility
			return nil, fmt.Errorf("failed to create policy metadata in PDP: %w, and failed to compensate: %w", err, deleteErr)
		}
		slog.Info("Successfully compensated schema creation", "schemaID", schema.SchemaID)
		return nil, fmt.Errorf("failed to create policy metadata in PDP: %w", err)
	}

	response := &models.SchemaResponse{
		SchemaID:   schema.SchemaID,
		SchemaName: schema.SchemaName,
		SDL:        schema.SDL,
		Endpoint:   schema.Endpoint,
		Version:    schema.Version,
		MemberID:   schema.MemberID,
		CreatedAt:  schema.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  schema.UpdatedAt.Format(time.RFC3339),
	}
	if schema.SchemaDescription != nil && *schema.SchemaDescription != "" {
		response.SchemaDescription = schema.SchemaDescription
	}

	return response, nil
}

// UpdateSchema updates an existing schema
func (s *SchemaService) UpdateSchema(schemaID string, req *models.UpdateSchemaRequest) (*models.SchemaResponse, error) {
	var schema models.Schema
	err := s.db.First(&schema, "schema_id = ?", schemaID).Error
	if err != nil {
		return nil, fmt.Errorf("schema not found: %w", err)
	}

	// Update fields if provided
	if req.SchemaName != nil {
		schema.SchemaName = *req.SchemaName
	}
	if req.SchemaDescription != nil {
		schema.SchemaDescription = req.SchemaDescription
	}
	if req.SDL != nil {
		schema.SDL = *req.SDL
	}
	if req.Endpoint != nil {
		schema.Endpoint = *req.Endpoint
	}
	if req.Version != nil {
		schema.Version = *req.Version
	}

	if err := s.db.Save(&schema).Error; err != nil {
		return nil, fmt.Errorf("failed to update schema: %w", err)
	}

	response := &models.SchemaResponse{
		SchemaID:   schema.SchemaID,
		SchemaName: schema.SchemaName,
		SDL:        schema.SDL,
		Endpoint:   schema.Endpoint,
		Version:    schema.Version,
		MemberID:   schema.MemberID,
		CreatedAt:  schema.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  schema.UpdatedAt.Format(time.RFC3339),
	}
	if schema.SchemaDescription != nil && *schema.SchemaDescription != "" {
		response.SchemaDescription = schema.SchemaDescription
	}

	return response, nil
}

// GetSchema retrieves a schema by ID
func (s *SchemaService) GetSchema(schemaID string) (*models.SchemaResponse, error) {
	var schema models.Schema
	err := s.db.First(&schema, "schema_id = ?", schemaID).Error
	if err != nil {
		return nil, fmt.Errorf("schema not found: %w", err)
	}

	response := &models.SchemaResponse{
		SchemaID:   schema.SchemaID,
		SchemaName: schema.SchemaName,
		SDL:        schema.SDL,
		Endpoint:   schema.Endpoint,
		Version:    schema.Version,
		MemberID:   schema.MemberID,
		CreatedAt:  schema.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  schema.UpdatedAt.Format(time.RFC3339),
	}
	if schema.SchemaDescription != nil && *schema.SchemaDescription != "" {
		response.SchemaDescription = schema.SchemaDescription
	}

	return response, nil
}

// GetSchemas Get all schemas and filter by member ID if given
func (s *SchemaService) GetSchemas(memberID *string) ([]*models.SchemaResponse, error) {
	var schemas []models.Schema
	query := s.db
	if memberID != nil && *memberID != "" {
		query = query.Where("member_id = ?", *memberID)
	}

	// Order by created_at descending
	query = query.Order("created_at DESC")

	err := query.Find(&schemas).Error
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve schemas: %w", err)
	}

	// Pre-allocate slice with known capacity for better performance
	responses := make([]*models.SchemaResponse, 0, len(schemas))
	for _, schema := range schemas {
		resp := &models.SchemaResponse{
			SchemaID:   schema.SchemaID,
			SchemaName: schema.SchemaName,
			SDL:        schema.SDL,
			Endpoint:   schema.Endpoint,
			Version:    schema.Version,
			MemberID:   schema.MemberID,
			CreatedAt:  schema.CreatedAt.Format(time.RFC3339),
			UpdatedAt:  schema.UpdatedAt.Format(time.RFC3339),
		}
		if schema.SchemaDescription != nil && *schema.SchemaDescription != "" {
			resp.SchemaDescription = schema.SchemaDescription
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

// CreateSchemaSubmission creates a new schema
func (s *SchemaService) CreateSchemaSubmission(req *models.CreateSchemaSubmissionRequest) (*models.SchemaSubmissionResponse, error) {
	// Check if member exists
	var member models.Member
	if err := s.db.First(&member, "member_id = ?", req.MemberID).Error; err != nil {
		return nil, fmt.Errorf("member not found: %w", err)
	}

	// If PreviousSchemaID is provided, check if it exists
	if req.PreviousSchemaID != nil {
		var previousSchema models.Schema
		if err := s.db.First(&previousSchema, "schema_id = ?", *req.PreviousSchemaID).Error; err != nil {
			return nil, fmt.Errorf("previous schema not found: %w", err)
		}
	}

	// Create submission
	submission := models.SchemaSubmission{
		SubmissionID:      "sub_" + uuid.New().String(),
		PreviousSchemaID:  req.PreviousSchemaID,
		SchemaName:        req.SchemaName,
		SchemaDescription: req.SchemaDescription,
		SDL:               req.SDL,
		SchemaEndpoint:    req.SchemaEndpoint,
		Status:            string(models.StatusPending),
		MemberID:          req.MemberID,
	}
	if err := s.db.Create(&submission).Error; err != nil {
		return nil, fmt.Errorf("failed to create schema submission: %w", err)
	}

	response := &models.SchemaSubmissionResponse{
		SubmissionID:      submission.SubmissionID,
		PreviousSchemaID:  submission.PreviousSchemaID,
		SchemaName:        submission.SchemaName,
		SchemaDescription: submission.SchemaDescription,
		SDL:               submission.SDL,
		SchemaEndpoint:    submission.SchemaEndpoint,
		Status:            submission.Status,
		MemberID:          submission.MemberID,
		CreatedAt:         submission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         submission.UpdatedAt.Format(time.RFC3339),
	}

	return response, nil
}

// UpdateSchemaSubmission updates an existing schema submission
func (s *SchemaService) UpdateSchemaSubmission(submissionID string, req *models.UpdateSchemaSubmissionRequest) (*models.SchemaSubmissionResponse, error) {
	var submission models.SchemaSubmission

	// Find the submission
	if err := s.db.First(&submission, "submission_id = ?", submissionID).Error; err != nil {
		return nil, fmt.Errorf("schema submission not found: %w", err)
	}

	// Validate PreviousSchemaID first before making any updates
	if req.PreviousSchemaID != nil {
		// Check if the new PreviousSchemaID exists
		var previousSchema models.Schema
		if err := s.db.First(&previousSchema, "schema_id = ?", *req.PreviousSchemaID).Error; err != nil {
			return nil, fmt.Errorf("previous schema not found: %w", err)
		}
	}

	// Update fields if provided
	if req.SchemaName != nil {
		submission.SchemaName = *req.SchemaName
	}
	if req.SchemaDescription != nil {
		submission.SchemaDescription = req.SchemaDescription
	}
	if req.SDL != nil {
		if *req.SDL == "" {
			return nil, fmt.Errorf("SDL field cannot be empty")
		}
		submission.SDL = *req.SDL
	}
	if req.SchemaEndpoint != nil {
		submission.SchemaEndpoint = *req.SchemaEndpoint
	}

	if req.PreviousSchemaID != nil {
		submission.PreviousSchemaID = req.PreviousSchemaID
	}

	var shouldCreateSchema bool
	if req.Status != nil {
		submission.Status = *req.Status
		// Mark that we need to create a schema after saving
		if *req.Status == string(models.StatusApproved) {
			shouldCreateSchema = true
		}
	}

	if req.Review != nil {
		submission.Review = req.Review
	}

	// Save the updated submission
	if err := s.db.Save(&submission).Error; err != nil {
		return nil, fmt.Errorf("failed to update schema submission: %w", err)
	}

	// Create schema outside of transaction if approval was successful
	if shouldCreateSchema {
		var createSchemaRequest models.CreateSchemaRequest
		createSchemaRequest.SchemaName = submission.SchemaName
		createSchemaRequest.SchemaDescription = submission.SchemaDescription
		createSchemaRequest.SDL = submission.SDL
		createSchemaRequest.Endpoint = submission.SchemaEndpoint
		createSchemaRequest.MemberID = submission.MemberID

		_, err := s.CreateSchema(&createSchemaRequest)
		if err != nil {
			// Compensation: Update submission status back to pending
			submission.Status = string(models.StatusPending)
			if updateErr := s.db.Save(&submission).Error; updateErr != nil {
				slog.Error("Failed to compensate submission status after schema creation failure",
					"submissionID", submission.SubmissionID,
					"originalError", err,
					"compensationError", updateErr)
				return nil, fmt.Errorf("failed to create schema from approved submission: %w, and failed to compensate submission status: %w", err, updateErr)
			}
			slog.Info("Successfully compensated submission status after schema creation failure", "submissionID", submission.SubmissionID)
			return nil, fmt.Errorf("failed to create schema from approved submission: %w", err)
		}
	}

	response := &models.SchemaSubmissionResponse{
		SubmissionID:      submission.SubmissionID,
		PreviousSchemaID:  submission.PreviousSchemaID,
		SchemaName:        submission.SchemaName,
		SchemaDescription: submission.SchemaDescription,
		SDL:               submission.SDL,
		SchemaEndpoint:    submission.SchemaEndpoint,
		Status:            submission.Status,
		MemberID:          submission.MemberID,
		CreatedAt:         submission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         submission.UpdatedAt.Format(time.RFC3339),
		Review:            submission.Review,
	}

	return response, nil
}

// GetSchemaSubmission retrieves a schema submission by ID
func (s *SchemaService) GetSchemaSubmission(submissionID string) (*models.SchemaSubmissionResponse, error) {
	var submission models.SchemaSubmission
	err := s.db.First(&submission, "submission_id = ?", submissionID).Error
	if err != nil {
		return nil, fmt.Errorf("schema submission not found: %w", err)
	}

	response := &models.SchemaSubmissionResponse{
		SubmissionID:      submission.SubmissionID,
		PreviousSchemaID:  submission.PreviousSchemaID,
		SchemaName:        submission.SchemaName,
		SchemaDescription: submission.SchemaDescription,
		SDL:               submission.SDL,
		SchemaEndpoint:    submission.SchemaEndpoint,
		Status:            submission.Status,
		MemberID:          submission.MemberID,
		CreatedAt:         submission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         submission.UpdatedAt.Format(time.RFC3339),
		Review:            submission.Review,
	}

	return response, nil
}

// GetSchemaSubmissions Get all schema submissions and filter by member ID OR Status Array if given
func (s *SchemaService) GetSchemaSubmissions(memberID *string, statusFilter *[]string) ([]*models.SchemaSubmissionResponse, error) {
	var submissions []models.SchemaSubmission
	query := s.db.Preload("PreviousSchema").Preload("Member")
	if memberID != nil && *memberID != "" {
		query = query.Where("member_id = ?", *memberID)
	}

	// Order by created_at descending
	query = query.Order("created_at DESC")

	if statusFilter != nil && len(*statusFilter) > 0 {
		query = query.Where("status IN ?", *statusFilter)
	}

	err := query.Find(&submissions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve schema submissions: %w", err)
	}

	var responses []*models.SchemaSubmissionResponse
	for _, submission := range submissions {
		responses = append(responses, &models.SchemaSubmissionResponse{
			SubmissionID:      submission.SubmissionID,
			PreviousSchemaID:  submission.PreviousSchemaID,
			SchemaName:        submission.SchemaName,
			SchemaDescription: submission.SchemaDescription,
			SDL:               submission.SDL,
			SchemaEndpoint:    submission.SchemaEndpoint,
			Status:            submission.Status,
			MemberID:          submission.MemberID,
			CreatedAt:         submission.CreatedAt.Format(time.RFC3339),
			UpdatedAt:         submission.UpdatedAt.Format(time.RFC3339),
			Review:            submission.Review,
		})
	}

	return responses, nil
}
