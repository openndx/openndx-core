package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/engine"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"gorm.io/gorm"
)

var (
	ErrPolicyMetadataNotFound = errors.New("policy metadata not found")
	ErrAccessGrantNotFound    = errors.New("active access grant not found")
	ErrInvalidGrantRequest    = errors.New("invalid access grant request")
	ErrUnknownPolicyClass     = errors.New("unknown policy class")
)

// PolicyMetadataService provides business logic for policy metadata operations
type PolicyMetadataService struct {
	db     *gorm.DB
	engine *engine.Engine
}

// NewPolicyMetadataService creates a new policy metadata service
func NewPolicyMetadataService(db *gorm.DB, policyEngine *engine.Engine) *PolicyMetadataService {
	return &PolicyMetadataService{
		db:     db,
		engine: policyEngine,
	}
}

// checkPolicyClass rejects a policy class the engine has not loaded, so a field
// is never stored with a class that would deny every request.
func (s *PolicyMetadataService) checkPolicyClass(class string) error {
	if !s.engine.HasClass(class) {
		return fmt.Errorf("%w %q: must be one of %s",
			ErrUnknownPolicyClass, class, strings.Join(s.engine.Classes(), ", "))
	}
	return nil
}

// CreatePolicyMetadata creates new policy metadata records with validation
func (s *PolicyMetadataService) CreatePolicyMetadata(req *models.PolicyMetadataCreateRequest) (*models.PolicyMetadataCreateResponse, error) {
	for _, record := range req.Records {
		if err := s.checkPolicyClass(record.PolicyClass); err != nil {
			return nil, fmt.Errorf("field %s: %w", record.FieldName, err)
		}
	}

	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Check if there are already records for the given schema ID
	var existingMetadata []models.PolicyMetadata
	if err := tx.Where("schema_id = ?", req.SchemaID).Find(&existingMetadata).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to check existing policy metadata: %w", err)
	}

	// Create a map for faster lookups of existing records by field name
	existingMap := make(map[string]*models.PolicyMetadata)
	for i := range existingMetadata {
		metadata := &existingMetadata[i]
		existingMap[metadata.FieldName] = metadata
	}

	now := time.Now()
	var newRecords []models.PolicyMetadata
	var updatedRecords []*models.PolicyMetadata
	processedFields := make(map[string]struct{})

	// Process incoming records (update in memory only)
	for _, record := range req.Records {
		processedFields[record.FieldName] = struct{}{}

		policyParams := record.PolicyParams
		if policyParams == nil {
			policyParams = make(models.PolicyParams)
		}

		if existing, exists := existingMap[record.FieldName]; exists {
			// Update existing record in memory
			existing.DisplayName = record.DisplayName
			existing.Description = record.Description
			existing.Source = record.Source
			existing.PolicyClass = record.PolicyClass
			existing.PolicyParams = policyParams
			existing.Owner = record.Owner
			existing.UpdatedAt = now

			updatedRecords = append(updatedRecords, existing)
		} else {
			// Prepare new record
			policyMetadata := models.PolicyMetadata{
				ID:           uuid.New(),
				SchemaID:     req.SchemaID,
				FieldName:    record.FieldName,
				DisplayName:  record.DisplayName,
				Description:  record.Description,
				Source:       record.Source,
				PolicyClass:  record.PolicyClass,
				PolicyParams: policyParams,
				Owner:        record.Owner,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			newRecords = append(newRecords, policyMetadata)
		}
	}

	// Delete records that weren't in the request (obsolete records)
	var idsToDelete []uuid.UUID
	for fieldName, existing := range existingMap {
		if _, processed := processedFields[fieldName]; !processed {
			idsToDelete = append(idsToDelete, existing.ID)
		}
	}

	if len(idsToDelete) > 0 {
		if err := tx.Where("id IN ?", idsToDelete).Delete(&models.PolicyMetadata{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to delete obsolete policy metadata records: %w", err)
		}
	}

	// Bulk create new records
	if len(newRecords) > 0 {
		if err := tx.Create(&newRecords).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create policy metadata records: %w", err)
		}
	}

	// Update only the metadata columns. Deleting obsolete records above also
	// deletes their access grants; grants of updated records are kept.
	for _, pm := range updatedRecords {
		if err := tx.Model(pm).
			Select(
				"display_name",
				"description",
				"source",
				"policy_class",
				"policy_params",
				"owner",
				"updated_at",
			).
			Updates(pm).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update existing policy metadata: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Prepare response including both new and updated records
	var responseRecords []models.PolicyMetadataResponse

	// Add new records to response
	for _, pm := range newRecords {
		responseRecords = append(responseRecords, pm.ToResponse())
	}

	// Add updated records to response
	for _, pm := range updatedRecords {
		responseRecords = append(responseRecords, pm.ToResponse())
	}

	return &models.PolicyMetadataCreateResponse{
		Records: responseRecords,
	}, nil
}

// GetPolicyDecision evaluates the policy for each requested field
func (s *PolicyMetadataService) GetPolicyDecision(ctx context.Context, req *models.PolicyDecisionRequest) (*models.PolicyDecisionResponse, error) {
	// Collect all unique schema IDs from the request
	schemaIDSet := make(map[string]struct{})
	for _, record := range req.RequiredFields {
		schemaIDSet[record.SchemaID] = struct{}{}
	}

	var schemaIDs []string
	for schemaID := range schemaIDSet {
		schemaIDs = append(schemaIDs, schemaID)
	}

	// Fetch all PolicyMetadata records for those schemas in one query
	var allMetadata []models.PolicyMetadata
	if err := s.db.Where("schema_id IN ?", schemaIDs).Find(&allMetadata).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch policy metadata records: %w", err)
	}

	// Create map for fast lookup: (schema_id + field_name) -> &PolicyMetadata
	metadataMap := make(map[string]*models.PolicyMetadata)
	for i := range allMetadata {
		pm := &allMetadata[i]
		key := pm.SchemaID + ":" + pm.FieldName
		metadataMap[key] = pm
	}

	// Load the application's active grants for those schemas. An expired grant
	// is still passed to the policy, which decides the field is expired.
	var grants []models.AccessGrant
	if err := s.db.
		Where("application_id = ? AND status = ? AND schema_id IN ?",
			req.ApplicationID, models.GrantStatusActive, schemaIDs).
		Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch access grants: %w", err)
	}
	grantMap := make(map[string]*models.AccessGrant, len(grants))
	for i := range grants {
		grantMap[grants[i].SchemaID+":"+grants[i].FieldName] = &grants[i]
	}

	fields := make([]models.PolicyDecisionFieldOutcome, 0, len(req.RequiredFields))
	var consentRequiredFields []models.PolicyDecisionResponseFieldRecord
	var unauthorizedFields []models.PolicyDecisionResponseFieldRecord
	var expiredFields []models.PolicyDecisionResponseFieldRecord

	for _, record := range req.RequiredFields {
		key := record.SchemaID + ":" + record.FieldName
		pm, exists := metadataMap[key]
		if !exists {
			return nil, fmt.Errorf("policy metadata not found for schema_id %s and field_name %s", record.SchemaID, record.FieldName)
		}

		input := engine.Input{
			App: engine.App{ID: req.ApplicationID},
			Field: engine.Field{
				SchemaID:    pm.SchemaID,
				FieldName:   pm.FieldName,
				PolicyClass: pm.PolicyClass,
				Params:      pm.PolicyParams,
			},
			Now: time.Now(),
		}
		if grant, ok := grantMap[key]; ok {
			input.Grant = &engine.Grant{ExpiresAt: grant.ExpiresAt}
		}

		// Decide denies the field when evaluation fails, so the error is only logged.
		decision, err := s.engine.Decide(ctx, input)
		if err != nil {
			slog.ErrorContext(ctx, "Policy evaluation failed, denying field",
				"schemaId", pm.SchemaID, "fieldName", pm.FieldName, "error", err)
		}

		fields = append(fields, models.PolicyDecisionFieldOutcome{
			SchemaID:    pm.SchemaID,
			FieldName:   pm.FieldName,
			Outcome:     string(decision.Outcome),
			Reason:      decision.Reason,
			Obligations: decision.Obligations,
		})

		fieldRecord := models.PolicyDecisionResponseFieldRecord{
			FieldName:   pm.FieldName,
			SchemaID:    pm.SchemaID,
			DisplayName: pm.DisplayName,
			Description: pm.Description,
			Owner:       pm.Owner,
		}
		switch decision.Outcome {
		case engine.OutcomeAllow:
		case engine.OutcomeConsentRequired:
			consentRequiredFields = append(consentRequiredFields, fieldRecord)
		case engine.OutcomeExpired:
			expiredFields = append(expiredFields, fieldRecord)
		default:
			unauthorizedFields = append(unauthorizedFields, fieldRecord)
		}
	}

	response := &models.PolicyDecisionResponse{
		Fields:                  fields,
		ConsentRequiredFields:   consentRequiredFields,
		UnauthorizedFields:      unauthorizedFields,
		ExpiredFields:           expiredFields,
		AppAuthorized:           len(unauthorizedFields) == 0,
		AppAccessExpired:        len(expiredFields) > 0,
		AppRequiresOwnerConsent: len(consentRequiredFields) > 0,
	}

	return response, nil
}

// ListPolicyMetadata returns policy metadata records, optionally filtered by schema.
func (s *PolicyMetadataService) ListPolicyMetadata(
	schemaID string,
) (*models.PolicyMetadataListResponse, error) {
	var records []models.PolicyMetadata
	query := s.db.Model(&models.PolicyMetadata{})

	if schemaID != "" {
		query = query.Where("schema_id = ?", schemaID)
	}

	if err := query.
		Order("schema_id ASC, field_name ASC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list policy metadata: %w", err)
	}

	responseRecords := make([]models.PolicyMetadataResponse, 0, len(records))
	for i := range records {
		responseRecords = append(responseRecords, records[i].ToResponse())
	}

	return &models.PolicyMetadataListResponse{
		Records: responseRecords,
	}, nil
}

// PatchPolicyMetadata updates selected properties of one policy metadata record.
func (s *PolicyMetadataService) PatchPolicyMetadata(
	id string,
	req *models.PolicyMetadataPatchRequest,
) (*models.PolicyMetadataResponse, error) {
	var policyMetadata models.PolicyMetadata
	updates := make(map[string]interface{})

	if err := s.db.First(&policyMetadata, "CAST(id AS TEXT) = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrPolicyMetadataNotFound, id)
		}

		return nil, fmt.Errorf("failed to fetch policy metadata: %w", err)
	}

	if req.DisplayName.Set {
		policyMetadata.DisplayName = req.DisplayName.Value
		updates["display_name"] = req.DisplayName.Value
	}

	if req.Description.Set {
		policyMetadata.Description = req.Description.Value
		updates["description"] = req.Description.Value
	}

	if req.PolicyClass.Set {
		if req.PolicyClass.Value == nil {
			return nil, fmt.Errorf("%w: policyClass cannot be null", ErrUnknownPolicyClass)
		}
		if err := s.checkPolicyClass(*req.PolicyClass.Value); err != nil {
			return nil, err
		}

		policyMetadata.PolicyClass = *req.PolicyClass.Value
		updates["policy_class"] = policyMetadata.PolicyClass
	}

	if req.PolicyParams.Set {
		// null clears the parameters
		policyParams := make(models.PolicyParams)
		if req.PolicyParams.Value != nil && *req.PolicyParams.Value != nil {
			policyParams = *req.PolicyParams.Value
		}

		policyMetadata.PolicyParams = policyParams
		updates["policy_params"] = policyParams
	}

	policyMetadata.UpdatedAt = time.Now()
	updates["updated_at"] = policyMetadata.UpdatedAt

	if err := s.db.Model(&policyMetadata).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update policy metadata: %w", err)
	}

	response := policyMetadata.ToResponse()
	return &response, nil
}

// DeletePolicyMetadata deletes one policy metadata record by ID.
func (s *PolicyMetadataService) DeletePolicyMetadata(id string) error {
	result := s.db.Where("CAST(id AS TEXT) = ?", id).Delete(&models.PolicyMetadata{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete policy metadata: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrPolicyMetadataNotFound, id)
	}

	return nil
}
