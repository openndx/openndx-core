package fieldpolicy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrPolicyMetadataNotFound = errors.New("policy metadata not found")
	ErrInvalidPolicy          = errors.New("invalid field policy")
)

// Service manages the field policy catalogue: the policy metadata of every
// field a schema exposes.
type Service struct {
	db *gorm.DB
}

// NewService creates a new field policy service
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreatePolicyMetadata replaces a schema's policy metadata with the request's
// records. Existing fields are updated in place and keep their access grants;
// fields missing from the request are deleted along with their grants.
func (s *Service) CreatePolicyMetadata(req *PolicyMetadataCreateRequest) (*PolicyMetadataCreateResponse, error) {
	// A missing flag is rejected rather than stored as false, which would open
	// the field.
	for _, record := range req.Records {
		if record.RequiresAllowlist == nil || record.RequiresConsent == nil {
			return nil, fmt.Errorf("%w: field %s needs requiresAllowlist and requiresConsent",
				ErrInvalidPolicy, record.FieldName)
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
	var existingMetadata []PolicyMetadata
	if err := tx.Where("schema_id = ?", req.SchemaID).Find(&existingMetadata).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to check existing policy metadata: %w", err)
	}

	// Create a map for faster lookups of existing records by field name
	existingMap := make(map[string]*PolicyMetadata)
	for i := range existingMetadata {
		metadata := &existingMetadata[i]
		existingMap[metadata.FieldName] = metadata
	}

	now := time.Now()
	var newRecords []PolicyMetadata
	var updatedRecords []*PolicyMetadata
	processedFields := make(map[string]struct{})

	// Process incoming records (update in memory only)
	for _, record := range req.Records {
		processedFields[record.FieldName] = struct{}{}

		if existing, exists := existingMap[record.FieldName]; exists {
			// Update existing record in memory
			existing.DisplayName = record.DisplayName
			existing.Description = record.Description
			existing.Source = record.Source
			existing.RequiresAllowlist = *record.RequiresAllowlist
			existing.RequiresConsent = *record.RequiresConsent
			existing.Owner = record.Owner
			existing.UpdatedAt = now

			updatedRecords = append(updatedRecords, existing)
		} else {
			// Prepare new record
			policyMetadata := PolicyMetadata{
				ID:                uuid.New(),
				SchemaID:          req.SchemaID,
				FieldName:         record.FieldName,
				DisplayName:       record.DisplayName,
				Description:       record.Description,
				Source:            record.Source,
				RequiresAllowlist: *record.RequiresAllowlist,
				RequiresConsent:   *record.RequiresConsent,
				Owner:             record.Owner,
				CreatedAt:         now,
				UpdatedAt:         now,
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
		if err := tx.Where("id IN ?", idsToDelete).Delete(&PolicyMetadata{}).Error; err != nil {
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
				"requires_allowlist",
				"requires_consent",
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
	var responseRecords []PolicyMetadataResponse

	// Add new records to response
	for _, pm := range newRecords {
		responseRecords = append(responseRecords, pm.ToResponse())
	}

	// Add updated records to response
	for _, pm := range updatedRecords {
		responseRecords = append(responseRecords, pm.ToResponse())
	}

	return &PolicyMetadataCreateResponse{
		Records: responseRecords,
	}, nil
}

// FieldKey names one field of one schema.
type FieldKey struct {
	SchemaID  string
	FieldName string
}

// FieldsByKeys returns the policy metadata of the named fields, in no
// particular order. A key with no policy metadata is left out of the result;
// the caller decides whether that is an error.
func (s *Service) FieldsByKeys(ctx context.Context, keys []FieldKey) ([]PolicyMetadata, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	pairs := make([][]any, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, []any{key.SchemaID, key.FieldName})
	}

	var fields []PolicyMetadata
	if err := s.db.WithContext(ctx).
		Where("(schema_id, field_name) IN ?", pairs).
		Find(&fields).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch policy metadata records: %w", err)
	}
	return fields, nil
}

// ListPolicyMetadata returns policy metadata records, optionally filtered by schema.
func (s *Service) ListPolicyMetadata(
	schemaID string,
) (*PolicyMetadataListResponse, error) {
	var records []PolicyMetadata
	query := s.db.Model(&PolicyMetadata{})

	if schemaID != "" {
		query = query.Where("schema_id = ?", schemaID)
	}

	if err := query.
		Order("schema_id ASC, field_name ASC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list policy metadata: %w", err)
	}

	responseRecords := make([]PolicyMetadataResponse, 0, len(records))
	for i := range records {
		responseRecords = append(responseRecords, records[i].ToResponse())
	}

	return &PolicyMetadataListResponse{
		Records: responseRecords,
	}, nil
}

// PatchPolicyMetadata updates selected properties of one policy metadata record.
func (s *Service) PatchPolicyMetadata(
	id string,
	req *PolicyMetadataPatchRequest,
) (*PolicyMetadataResponse, error) {
	var policyMetadata PolicyMetadata
	updates := make(map[string]any)

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

	if flag := req.NullFlag(); flag != "" {
		return nil, fmt.Errorf("%w: %s cannot be null", ErrInvalidPolicy, flag)
	}

	if req.RequiresAllowlist.Set {
		policyMetadata.RequiresAllowlist = *req.RequiresAllowlist.Value
		updates["requires_allowlist"] = policyMetadata.RequiresAllowlist
	}

	if req.RequiresConsent.Set {
		policyMetadata.RequiresConsent = *req.RequiresConsent.Value
		updates["requires_consent"] = policyMetadata.RequiresConsent
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
func (s *Service) DeletePolicyMetadata(id string) error {
	result := s.db.Where("CAST(id AS TEXT) = ?", id).Delete(&PolicyMetadata{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete policy metadata: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrPolicyMetadataNotFound, id)
	}

	return nil
}
