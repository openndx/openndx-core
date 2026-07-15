package services

import (
	"fmt"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/policy-decision-point/v1/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
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

	// Fetch all matching PolicyMetadata records in one query
	var policyMetadataRecords []models.PolicyMetadata
	whereClause := "(" + conditions[0]
	for i := 1; i < len(conditions); i++ {
		whereClause += " OR " + conditions[i]
	}
	whereClause += ")"

	if err := s.db.Where(whereClause, args...).Find(&policyMetadataRecords).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch policy metadata records: %w", err)
	}

	// Create map for fast lookup: (schema_id + field_name) -> &PolicyMetadata
	policyMap := make(map[string]*models.PolicyMetadata)
	for i := range policyMetadataRecords {
		pm := &policyMetadataRecords[i]
		key := pm.SchemaID + ":" + pm.FieldName
		policyMap[key] = pm
	}

	// Check if all requested records exist
	for key := range requestMap {
		if _, exists := policyMap[key]; !exists {
			record := requestMap[key]
			return nil, fmt.Errorf("policy metadata not found for schema_id %s and field_name %s", record.SchemaID, record.FieldName)
		}
	}

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

	var responseRecords []models.AllowListUpdateResponseRecord
	var recordsToUpdate []*models.PolicyMetadata

	// Update records in memory first
	for _, record := range req.Records {
		key := record.SchemaID + ":" + record.FieldName
		pm := policyMap[key]

		// Update allow list
		if pm.AllowList == nil {
			pm.AllowList = make(models.AllowList)
		}
		pm.AllowList[req.ApplicationID] = models.AllowListEntry{
			ExpiresAt: expiresAt,
			UpdatedAt: currentTime,
		}

		recordsToUpdate = append(recordsToUpdate, pm)

		// Prepare response record
		responseRecord := models.AllowListUpdateResponseRecord{
			FieldName: record.FieldName,
			SchemaID:  record.SchemaID,
			ExpiresAt: expiresAt.Format(time.RFC3339),
			UpdatedAt: currentTime.Format(time.RFC3339),
		}
		responseRecords = append(responseRecords, responseRecord)
	}

	// Update all records
	// Note: We perform individual updates because each record has a different allow_list value.
	// Each field's allow_list map may already contain entries for other applications, so individual
	// updates are necessary to ensure the correct application ID and expiration time are set for each field.
	// However, this function only updates the allow_list for a single application ID and expiration time
	// per request (from req.ApplicationID and req.GrantDuration); all records in the batch receive the same values.
	// The custom AllowList type's Value() method ensures proper JSONB serialization for each record.
	if len(recordsToUpdate) > 0 {
		for _, pm := range recordsToUpdate {
			pm.UpdatedAt = currentTime
			if err := tx.Model(pm).Select("allow_list", "updated_at").Updates(map[string]interface{}{
				"allow_list": pm.AllowList,
				"updated_at": pm.UpdatedAt,
			}).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("failed to update allow list record: %w", err)
			}
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
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
