package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrPolicyMetadataNotFound = errors.New("policy metadata not found")
	ErrAllowListEntryNotFound = errors.New("allow-list entry not found")
)

// PolicyMetadataService provides business logic for policy metadata operations
type PolicyMetadataService struct {
	db *gorm.DB
}

// NewPolicyMetadataService creates a new policy metadata service
func NewPolicyMetadataService(db *gorm.DB) *PolicyMetadataService {
	return &PolicyMetadataService{
		db: db,
	}
}

// CreatePolicyMetadata creates new policy metadata records with validation
func (s *PolicyMetadataService) CreatePolicyMetadata(req *models.PolicyMetadataCreateRequest) (*models.PolicyMetadataCreateResponse, error) {
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

		if existing, exists := existingMap[record.FieldName]; exists {
			// Update existing record in memory
			existing.DisplayName = record.DisplayName
			existing.Description = record.Description
			existing.Source = record.Source
			existing.IsOwner = record.IsOwner
			existing.AccessControlType = record.AccessControlType
			existing.Owner = record.Owner
			existing.UpdatedAt = now

			updatedRecords = append(updatedRecords, existing)
		} else {
			// Prepare new record
			policyMetadata := models.PolicyMetadata{
				ID:                uuid.New(),
				SchemaID:          req.SchemaID,
				FieldName:         record.FieldName,
				DisplayName:       record.DisplayName,
				Description:       record.Description,
				Source:            record.Source,
				IsOwner:           record.IsOwner,
				AccessControlType: record.AccessControlType,
				AllowList:         make(models.AllowList),
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

	// Bulk save updated records
	if len(updatedRecords) > 0 {
		// Convert slice of pointers to slice of values for batch update
		var recordsToUpdate []models.PolicyMetadata
		for _, pm := range updatedRecords {
			recordsToUpdate = append(recordsToUpdate, *pm)
		}

		if err := tx.Save(&recordsToUpdate).Error; err != nil {
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

// UpdateAllowList updates the allow list for multiple fields with validation
func (s *PolicyMetadataService) UpdateAllowList(req *models.AllowListUpdateRequest) (*models.AllowListUpdateResponse, error) {
	// Collect all (schema_id, field_name) pairs from the request
	var conditions []string
	var args []interface{}
	requestMap := make(map[string]*models.AllowListUpdateRequestRecord)

	for i := range req.Records {
		record := &req.Records[i]
		key := record.SchemaID + ":" + record.FieldName
		requestMap[key] = record

		conditions = append(conditions, "(schema_id = ? AND field_name = ?)")
		args = append(args, record.SchemaID, record.FieldName)
	}

	if len(conditions) == 0 {
		return &models.AllowListUpdateResponse{Records: []models.AllowListUpdateResponseRecord{}}, nil
	}

	whereClause := "(" + conditions[0]
	for i := 1; i < len(conditions); i++ {
		whereClause += " OR " + conditions[i]
	}
	whereClause += ")"

	// Calculate expiration time based on grant duration
	currentTime := time.Now()
	var expiresAt time.Time
	switch req.GrantDuration {
	case models.GrantDurationTypeOneMonth:
		expiresAt = currentTime.AddDate(0, 1, 0)
	case models.GrantDurationTypeOneYear:
		expiresAt = currentTime.AddDate(1, 0, 0)
	default:
		return nil, fmt.Errorf("invalid grant duration: %s", req.GrantDuration)
	}

	var responseRecords []models.AllowListUpdateResponseRecord

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// Lock the matching rows before reading their allow-lists. Granting and
		// revoking therefore cannot overwrite each other with stale map values.
		var policyMetadataRecords []models.PolicyMetadata
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(whereClause, args...).
			Order("schema_id ASC, field_name ASC").
			Find(&policyMetadataRecords).Error; err != nil {
			return fmt.Errorf("failed to fetch policy metadata records: %w", err)
		}

		policyMap := make(map[string]*models.PolicyMetadata)
		for i := range policyMetadataRecords {
			pm := &policyMetadataRecords[i]
			key := pm.SchemaID + ":" + pm.FieldName
			policyMap[key] = pm
		}

		for key := range requestMap {
			if _, exists := policyMap[key]; !exists {
				record := requestMap[key]
				return fmt.Errorf(
					"policy metadata not found for schema_id %s and field_name %s",
					record.SchemaID,
					record.FieldName,
				)
			}
		}

		for _, record := range req.Records {
			key := record.SchemaID + ":" + record.FieldName
			pm := policyMap[key]

			if pm.AllowList == nil {
				pm.AllowList = make(models.AllowList)
			}
			pm.AllowList[req.ApplicationID] = models.AllowListEntry{
				ExpiresAt: expiresAt,
				UpdatedAt: currentTime,
			}
			pm.UpdatedAt = currentTime

			if err := tx.Model(pm).Select("allow_list", "updated_at").Updates(map[string]interface{}{
				"allow_list": pm.AllowList,
				"updated_at": pm.UpdatedAt,
			}).Error; err != nil {
				return fmt.Errorf("failed to update allow list record: %w", err)
			}

			responseRecords = append(responseRecords, models.AllowListUpdateResponseRecord{
				FieldName: record.FieldName,
				SchemaID:  record.SchemaID,
				ExpiresAt: expiresAt.Format(time.RFC3339),
				UpdatedAt: currentTime.Format(time.RFC3339),
			})
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &models.AllowListUpdateResponse{
		Records: responseRecords,
	}, nil
}

// GetPolicyDecision evaluates policy decision based on policy metadata
func (s *PolicyMetadataService) GetPolicyDecision(req *models.PolicyDecisionRequest) (*models.PolicyDecisionResponse, error) {
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

	var consentRequiredFields []models.PolicyDecisionResponseFieldRecord
	var unauthorizedFields []models.PolicyDecisionResponseFieldRecord
	var expiredFields []models.PolicyDecisionResponseFieldRecord

	// Iterate through required fields and perform logic using map lookup
	for _, record := range req.RequiredFields {
		key := record.SchemaID + ":" + record.FieldName
		pm, exists := metadataMap[key]
		if !exists {
			return nil, fmt.Errorf("policy metadata not found for schema_id %s and field_name %s", record.SchemaID, record.FieldName)
		}

		// Check if application is authorized
		if _, exists := pm.AllowList[req.ApplicationID]; !exists {
			unauthorizedFields = append(unauthorizedFields, models.PolicyDecisionResponseFieldRecord{
				FieldName:   pm.FieldName,
				SchemaID:    pm.SchemaID,
				DisplayName: pm.DisplayName,
				Description: pm.Description,
				Owner:       pm.Owner,
			})
			continue
		}

		// Check if access has expired
		allowListEntry := pm.AllowList[req.ApplicationID]
		if time.Now().After(allowListEntry.ExpiresAt) {
			expiredFields = append(expiredFields, models.PolicyDecisionResponseFieldRecord{
				FieldName:   pm.FieldName,
				SchemaID:    pm.SchemaID,
				DisplayName: pm.DisplayName,
				Description: pm.Description,
				Owner:       pm.Owner,
			})
			continue
		}

		// Check if owner consent is required
		if !pm.IsOwner && pm.AccessControlType == models.AccessControlTypeRestricted {
			consentRequiredFields = append(consentRequiredFields, models.PolicyDecisionResponseFieldRecord{
				FieldName:   pm.FieldName,
				SchemaID:    pm.SchemaID,
				DisplayName: pm.DisplayName,
				Description: pm.Description,
				Owner:       pm.Owner,
			})
		}
	}

	response := &models.PolicyDecisionResponse{
		ConsentRequiredFields:   consentRequiredFields,
		UnauthorizedFields:      unauthorizedFields,
		ExpiredFields:           expiredFields,
		AppAuthorized:           len(unauthorizedFields) == 0,
		AppAccessExpired:        len(expiredFields) > 0,
		AppRequiresOwnerConsent: len(consentRequiredFields) > 0,
	}

	return response, nil
}

// ListPolicyMetadata returns all policy metadata records for one schema.
func (s *PolicyMetadataService) ListPolicyMetadata(
	schemaID string,
) (*models.PolicyMetadataListResponse, error) {
	var records []models.PolicyMetadata

	if err := s.db.
		Where("schema_id = ?", schemaID).
		Order("field_name ASC").
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
	id uuid.UUID,
	req *models.PolicyMetadataPatchRequest,
) (*models.PolicyMetadataResponse, error) {
	var policyMetadata models.PolicyMetadata
	updates := make(map[string]interface{})

	if err := s.db.First(&policyMetadata, "id = ?", id).Error; err != nil {
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

	if req.AccessControlType.Set {
		if req.AccessControlType.Value == nil {
			return nil, errors.New("accessControlType cannot be null")
		}

		accessControlType := *req.AccessControlType.Value
		if accessControlType != models.AccessControlTypePublic &&
			accessControlType != models.AccessControlTypeRestricted {
			return nil, fmt.Errorf(
				"invalid accessControlType: %s",
				accessControlType,
			)
		}

		policyMetadata.AccessControlType = accessControlType
		updates["access_control_type"] = accessControlType
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
func (s *PolicyMetadataService) DeletePolicyMetadata(id uuid.UUID) error {
	result := s.db.Delete(&models.PolicyMetadata{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete policy metadata: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrPolicyMetadataNotFound, id)
	}

	return nil
}

// RevokeAllowListEntry removes one application from one field's allow-list.
func (s *PolicyMetadataService) RevokeAllowListEntry(
	id uuid.UUID,
	applicationID string,
) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var policyMetadata models.PolicyMetadata

		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&policyMetadata, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: %s", ErrPolicyMetadataNotFound, id)
			}

			return fmt.Errorf("failed to fetch policy metadata: %w", err)
		}

		if _, exists := policyMetadata.AllowList[applicationID]; !exists {
			return fmt.Errorf(
				"%w: %s",
				ErrAllowListEntryNotFound,
				applicationID,
			)
		}

		delete(policyMetadata.AllowList, applicationID)
		policyMetadata.UpdatedAt = time.Now()

		if err := tx.Model(&policyMetadata).
			Select("allow_list", "updated_at").
			Updates(map[string]interface{}{
				"allow_list": policyMetadata.AllowList,
				"updated_at": policyMetadata.UpdatedAt,
			}).Error; err != nil {
			return fmt.Errorf("failed to revoke allow-list entry: %w", err)
		}

		return nil
	})
}
