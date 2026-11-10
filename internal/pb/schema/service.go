package schema

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"gorm.io/gorm"
)

// ErrPolicyMetadataNotFound is returned when a policy metadata record does not
// exist in the requested schema.
var ErrPolicyMetadataNotFound = errors.New("policy metadata not found")

// ErrAllowListEntryNotFound is returned when an application holds no active
// grant for a field.
var ErrAllowListEntryNotFound = errors.New("allow-list entry not found")

var (
	// ErrInvalidSchemaID is returned when a caller-supplied schema ID doesn't
	// match schemaIDPattern.
	ErrInvalidSchemaID = errors.New("invalid schemaId: use 3-64 lowercase letters, digits, '.', '_' or '-', starting with a letter or digit")
	// ErrSchemaAlreadyExists is returned when a caller-supplied schema ID is
	// already taken.
	ErrSchemaAlreadyExists = errors.New("a schema with this schemaId already exists")
)

// schemaIDPattern keeps caller-supplied schema IDs safe in URL paths and in
// `schemaId:fieldName` references (no ':' or '/').
var schemaIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

// Service handles schema-related operations
type Service struct {
	db            *gorm.DB
	policyService *policy.Client
}

// NewService creates a new schema service
func NewService(db *gorm.DB, policyService *policy.Client) *Service {
	return &Service{db: db, policyService: policyService}
}

// CreateSchema creates a new schema. Exactly one of req.SDL / req.Fields must
// be set: SDL is parsed into policy metadata records via GraphQL directives,
// while Fields are used directly, skipping that parsing entirely.
//
// req.SchemaID, if set, becomes the schema's ID; otherwise a UUID is
// generated. A fixed ID lets the Orchestration Engine's SDL and config name the
// schema (`@sourceInfo(schemaId: ...)`) the same way in every environment.
func (s *Service) CreateSchema(req *CreateSchemaRequest) (*SchemaResponse, error) {
	hasSDL := req.SDL != ""
	hasFields := len(req.Fields) > 0
	if hasSDL == hasFields {
		return nil, fmt.Errorf("exactly one of sdl or fields must be provided")
	}

	schemaID := uuid.New().String()
	if req.SchemaID != "" {
		if !schemaIDPattern.MatchString(req.SchemaID) {
			return nil, ErrInvalidSchemaID
		}
		schemaID = req.SchemaID
	}

	schema := Schema{
		SchemaID:   schemaID,
		SchemaName: req.SchemaName,
		SDL:        req.SDL,
		Endpoint:   req.Endpoint,
		MemberID:   req.MemberID,
		Version:    string(kernel.ActiveVersion),
	}
	if req.SchemaDescription != nil {
		schema.SchemaDescription = req.SchemaDescription
	}

	// Step 1: Create schema in database first
	if err := s.db.Create(&schema).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrSchemaAlreadyExists
		}
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Step 2: Create policy metadata in PDP (Saga Pattern)
	var err error
	if hasFields {
		_, err = s.policyService.CreatePolicyMetadataFromRecords(schema.SchemaID, req.Fields)
	} else {
		_, err = s.policyService.CreatePolicyMetadata(schema.SchemaID, schema.SDL)
	}
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

	response := &SchemaResponse{
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
func (s *Service) UpdateSchema(schemaID string, req *UpdateSchemaRequest) (*SchemaResponse, error) {
	var schema Schema
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

	response := &SchemaResponse{
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
func (s *Service) GetSchema(schemaID string) (*SchemaResponse, error) {
	var schema Schema
	err := s.db.First(&schema, "schema_id = ?", schemaID).Error
	if err != nil {
		return nil, fmt.Errorf("schema not found: %w", err)
	}

	response := &SchemaResponse{
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
func (s *Service) GetSchemas(memberID *string) ([]*SchemaResponse, error) {
	var schemas []Schema
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
	responses := make([]*SchemaResponse, 0, len(schemas))
	for _, schema := range schemas {
		resp := &SchemaResponse{
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

// ListPolicyMetadata lists the PDP policy metadata records of a schema, each
// with its allow-list: the applications holding an active grant for the field.
func (s *Service) ListPolicyMetadata(schemaID string) (*policy.PolicyMetadataListResponse, error) {
	list, err := s.policyService.ListPolicyMetadata(schemaID)
	if err != nil {
		return nil, err
	}
	if err := s.attachAllowLists(schemaID, list.Records); err != nil {
		return nil, err
	}
	return list, nil
}

// PatchPolicyMetadata updates selected properties of one of a schema's policy metadata records
func (s *Service) PatchPolicyMetadata(schemaID, id string, req *policy.PolicyMetadataPatchRequest) (*policy.PolicyMetadataResponse, error) {
	if _, err := s.findPolicyMetadataInSchema(schemaID, id); err != nil {
		return nil, err
	}
	record, err := s.policyService.PatchPolicyMetadata(id, req)
	if err != nil {
		return nil, err
	}
	records := []policy.PolicyMetadataResponse{*record}
	if err := s.attachAllowLists(schemaID, records); err != nil {
		return nil, err
	}
	return &records[0], nil
}

// DeletePolicyMetadata deletes one of a schema's policy metadata records
func (s *Service) DeletePolicyMetadata(schemaID, id string) error {
	if _, err := s.findPolicyMetadataInSchema(schemaID, id); err != nil {
		return err
	}
	return s.policyService.DeletePolicyMetadata(id)
}

// RevokeAllowListEntry removes one application from the allow-list of one of a
// schema's fields by revoking the application's active grant for the field.
func (s *Service) RevokeAllowListEntry(schemaID, id, applicationID string) error {
	record, err := s.findPolicyMetadataInSchema(schemaID, id)
	if err != nil {
		return err
	}

	plain := false
	grants, err := s.policyService.ListAccessGrants(policy.AccessGrantFilter{
		ApplicationID: applicationID,
		SchemaID:      schemaID,
		Status:        policy.GrantStatusActive,
		ConsentBypass: &plain,
	})
	if err != nil {
		return err
	}
	for _, grant := range grants.Records {
		if grant.FieldName == record.FieldName {
			_, err := s.policyService.RevokeAccessGrant(grant.ID)
			return err
		}
	}

	return fmt.Errorf("%w: %s", ErrAllowListEntryNotFound, applicationID)
}

// attachAllowLists sets each record's allow-list from the schema's active
// plain access grants in the PDP. Consent bypass grants are managed through
// the consent bypass API and are not part of the allow-list.
func (s *Service) attachAllowLists(schemaID string, records []policy.PolicyMetadataResponse) error {
	plain := false
	grants, err := s.policyService.ListAccessGrants(policy.AccessGrantFilter{
		SchemaID:      schemaID,
		Status:        policy.GrantStatusActive,
		ConsentBypass: &plain,
	})
	if err != nil {
		return err
	}

	byField := make(map[string]policy.AllowList)
	for _, grant := range grants.Records {
		entry, err := grant.AllowListEntry()
		if err != nil {
			return err
		}
		if byField[grant.FieldName] == nil {
			byField[grant.FieldName] = make(policy.AllowList)
		}
		byField[grant.FieldName][grant.ApplicationID] = entry
	}

	for i := range records {
		allowList := byField[records[i].FieldName]
		if allowList == nil {
			allowList = make(policy.AllowList)
		}
		records[i].AllowList = allowList
	}
	return nil
}

// findPolicyMetadataInSchema returns the policy metadata record if it belongs
// to the schema, so that ownership of one schema cannot be used to change the
// policy metadata of another.
func (s *Service) findPolicyMetadataInSchema(schemaID, id string) (*policy.PolicyMetadataResponse, error) {
	list, err := s.policyService.ListPolicyMetadata(schemaID)
	if err != nil {
		return nil, err
	}

	for i := range list.Records {
		if list.Records[i].ID == id {
			return &list.Records[i], nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrPolicyMetadataNotFound, id)
}

// CreateSchemaSubmission creates a new schema
func (s *Service) CreateSchemaSubmission(req *CreateSchemaSubmissionRequest) (*SchemaSubmissionResponse, error) {
	// Check if member exists
	var owner member.Member
	if err := s.db.First(&owner, "member_id = ?", req.MemberID).Error; err != nil {
		return nil, fmt.Errorf("member not found: %w", err)
	}

	// If PreviousSchemaID is provided, check if it exists
	if req.PreviousSchemaID != nil {
		var previousSchema Schema
		if err := s.db.First(&previousSchema, "schema_id = ?", *req.PreviousSchemaID).Error; err != nil {
			return nil, fmt.Errorf("previous schema not found: %w", err)
		}
	}

	// Create submission
	submission := SchemaSubmission{
		SubmissionID:      uuid.New().String(),
		PreviousSchemaID:  req.PreviousSchemaID,
		SchemaName:        req.SchemaName,
		SchemaDescription: req.SchemaDescription,
		SDL:               req.SDL,
		SchemaEndpoint:    req.SchemaEndpoint,
		Status:            string(kernel.StatusPending),
		MemberID:          req.MemberID,
	}
	if err := s.db.Create(&submission).Error; err != nil {
		return nil, fmt.Errorf("failed to create schema submission: %w", err)
	}

	response := &SchemaSubmissionResponse{
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
func (s *Service) UpdateSchemaSubmission(submissionID string, req *UpdateSchemaSubmissionRequest) (*SchemaSubmissionResponse, error) {
	var submission SchemaSubmission

	// Find the submission
	if err := s.db.First(&submission, "submission_id = ?", submissionID).Error; err != nil {
		return nil, fmt.Errorf("schema submission not found: %w", err)
	}

	// Validate PreviousSchemaID first before making any updates
	if req.PreviousSchemaID != nil {
		// Check if the new PreviousSchemaID exists
		var previousSchema Schema
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
		if *req.Status == string(kernel.StatusApproved) {
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
		var createSchemaRequest CreateSchemaRequest
		createSchemaRequest.SchemaName = submission.SchemaName
		createSchemaRequest.SchemaDescription = submission.SchemaDescription
		createSchemaRequest.SDL = submission.SDL
		createSchemaRequest.Endpoint = submission.SchemaEndpoint
		createSchemaRequest.MemberID = submission.MemberID

		_, err := s.CreateSchema(&createSchemaRequest)
		if err != nil {
			// Compensation: Update submission status back to pending
			submission.Status = string(kernel.StatusPending)
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

	response := &SchemaSubmissionResponse{
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
func (s *Service) GetSchemaSubmission(submissionID string) (*SchemaSubmissionResponse, error) {
	var submission SchemaSubmission
	err := s.db.First(&submission, "submission_id = ?", submissionID).Error
	if err != nil {
		return nil, fmt.Errorf("schema submission not found: %w", err)
	}

	response := &SchemaSubmissionResponse{
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
func (s *Service) GetSchemaSubmissions(memberID *string, statusFilter *[]string) ([]*SchemaSubmissionResponse, error) {
	var submissions []SchemaSubmission
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

	var responses []*SchemaSubmissionResponse
	for _, submission := range submissions {
		responses = append(responses, &SchemaSubmissionResponse{
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
