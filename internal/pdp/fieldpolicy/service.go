package fieldpolicy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/decision"
	"gorm.io/gorm"
)

var (
	// ErrPolicyNotFound keeps the text clients of the field policy API match
	// on until the API itself is renamed.
	ErrPolicyNotFound = errors.New("policy metadata not found")
	ErrInvalidPolicy  = errors.New("invalid field policy")
)

// Service manages the field policy catalogue: the policy of every field a
// schema exposes.
type Service struct {
	db *gorm.DB
}

// NewService creates a new field policy service
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreatePolicies replaces a schema's field policies with the request's
// records. Existing fields are updated in place and keep their access grants;
// fields missing from the request are deleted along with their grants.
func (s *Service) CreatePolicies(req *PolicyCreateRequest) (*PolicyCreateResponse, error) {
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
	var existingPolicies []Policy
	if err := tx.Where("schema_id = ?", req.SchemaID).Find(&existingPolicies).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to check existing field policies: %w", err)
	}

	// Create a map for faster lookups of existing records by field name
	existingMap := make(map[string]*Policy)
	for i := range existingPolicies {
		policy := &existingPolicies[i]
		existingMap[policy.FieldName] = policy
	}

	now := time.Now()
	var newRecords []Policy
	var updatedRecords []*Policy
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
			policy := Policy{
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
			newRecords = append(newRecords, policy)
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
		if err := tx.Where("id IN ?", idsToDelete).Delete(&Policy{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to delete obsolete field policies: %w", err)
		}
	}

	// Bulk create new records
	if len(newRecords) > 0 {
		if err := tx.Create(&newRecords).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create field policies: %w", err)
		}
	}

	// Update only the policy columns. Deleting obsolete records above also
	// deletes their access grants; grants of updated records are kept.
	for _, p := range updatedRecords {
		if err := tx.Model(p).
			Select(
				"display_name",
				"description",
				"source",
				"requires_allowlist",
				"requires_consent",
				"owner",
				"updated_at",
			).
			Updates(p).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update existing field policies: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Prepare response including both new and updated records
	var responseRecords []PolicyResponse

	// Add new records to response
	for _, p := range newRecords {
		responseRecords = append(responseRecords, p.ToResponse())
	}

	// Add updated records to response
	for _, p := range updatedRecords {
		responseRecords = append(responseRecords, p.ToResponse())
	}

	return &PolicyCreateResponse{
		Records: responseRecords,
	}, nil
}

// FieldPolicies returns the policies of the named fields, in no particular
// order, for policy decisions. A key with no field policy is left out of the
// result; the caller decides whether that is an error.
func (s *Service) FieldPolicies(ctx context.Context, keys []decision.FieldKey) ([]decision.FieldPolicy, error) {
	fields, err := s.fieldsByKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	policies := make([]decision.FieldPolicy, 0, len(fields))
	for i := range fields {
		policies = append(policies, fields[i].toFieldPolicy())
	}
	return policies, nil
}

// fieldsByKeys returns the policies of the named fields, in no
// particular order.
func (s *Service) fieldsByKeys(ctx context.Context, keys []decision.FieldKey) ([]Policy, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	pairs := make([][]any, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, []any{key.SchemaID, key.FieldName})
	}

	var fields []Policy
	if err := s.db.WithContext(ctx).
		Where("(schema_id, field_name) IN ?", pairs).
		Find(&fields).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch field policies: %w", err)
	}
	return fields, nil
}

// ListPolicies returns field policies, optionally filtered by schema.
func (s *Service) ListPolicies(
	schemaID string,
) (*PolicyListResponse, error) {
	var records []Policy
	query := s.db.Model(&Policy{})

	if schemaID != "" {
		query = query.Where("schema_id = ?", schemaID)
	}

	if err := query.
		Order("schema_id ASC, field_name ASC").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list field policies: %w", err)
	}

	responseRecords := make([]PolicyResponse, 0, len(records))
	for i := range records {
		responseRecords = append(responseRecords, records[i].ToResponse())
	}

	return &PolicyListResponse{
		Records: responseRecords,
	}, nil
}

// PatchPolicy updates selected properties of one field policy.
func (s *Service) PatchPolicy(
	id string,
	req *PolicyPatchRequest,
) (*PolicyResponse, error) {
	var policy Policy
	updates := make(map[string]any)

	if err := s.db.First(&policy, "CAST(id AS TEXT) = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrPolicyNotFound, id)
		}

		return nil, fmt.Errorf("failed to fetch field policy: %w", err)
	}

	if req.DisplayName.Set {
		policy.DisplayName = req.DisplayName.Value
		updates["display_name"] = req.DisplayName.Value
	}

	if req.Description.Set {
		policy.Description = req.Description.Value
		updates["description"] = req.Description.Value
	}

	if flag := req.NullFlag(); flag != "" {
		return nil, fmt.Errorf("%w: %s cannot be null", ErrInvalidPolicy, flag)
	}

	if req.RequiresAllowlist.Set {
		policy.RequiresAllowlist = *req.RequiresAllowlist.Value
		updates["requires_allowlist"] = policy.RequiresAllowlist
	}

	if req.RequiresConsent.Set {
		policy.RequiresConsent = *req.RequiresConsent.Value
		updates["requires_consent"] = policy.RequiresConsent
	}

	policy.UpdatedAt = time.Now()
	updates["updated_at"] = policy.UpdatedAt

	if err := s.db.Model(&policy).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update field policy: %w", err)
	}

	response := policy.ToResponse()
	return &response, nil
}

// DeletePolicy deletes one field policy by ID.
func (s *Service) DeletePolicy(id string) error {
	result := s.db.Where("CAST(id AS TEXT) = ?", id).Delete(&Policy{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete field policy: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrPolicyNotFound, id)
	}

	return nil
}
