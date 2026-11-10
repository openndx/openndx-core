package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateAccessGrants grants an application access to fields for the requested
// duration. A field the application already holds an active grant for has that
// grant's expiry renewed. Every field must have policy metadata.
func (s *PolicyMetadataService) CreateAccessGrants(req *models.AccessGrantCreateRequest) (*models.AccessGrantListResponse, error) {
	if strings.TrimSpace(req.ApplicationID) == "" {
		return nil, fmt.Errorf("%w: applicationId is required", ErrInvalidGrantRequest)
	}
	for _, record := range req.Records {
		if record.SchemaID == "" || record.FieldName == "" {
			return nil, fmt.Errorf("%w: every record needs a schemaId and a fieldName", ErrInvalidGrantRequest)
		}
	}

	now := time.Now().UTC()
	expiresAt, ok := req.GrantDuration.ExpiresAt(now)
	if !ok {
		return nil, fmt.Errorf("%w: invalid grant duration %q", ErrInvalidGrantRequest, req.GrantDuration)
	}

	records := make([]models.AccessGrantResponse, 0, len(req.Records))
	if len(req.Records) == 0 {
		return &models.AccessGrantListResponse{Records: records}, nil
	}

	fields := make([][]any, 0, len(req.Records))
	for _, record := range req.Records {
		fields = append(fields, []any{record.SchemaID, record.FieldName})
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Lock the fields' policy metadata so concurrent grants for the same
		// field are serialized and cannot both create an active grant.
		var metadata []models.PolicyMetadata
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("(schema_id, field_name) IN ?", fields).
			Order("schema_id ASC, field_name ASC").
			Find(&metadata).Error; err != nil {
			return fmt.Errorf("failed to fetch policy metadata records: %w", err)
		}

		known := make(map[string]struct{}, len(metadata))
		for _, pm := range metadata {
			known[pm.SchemaID+":"+pm.FieldName] = struct{}{}
		}
		for _, record := range req.Records {
			if _, ok := known[record.SchemaID+":"+record.FieldName]; !ok {
				return fmt.Errorf("%w for schema_id %s and field_name %s",
					ErrPolicyMetadataNotFound, record.SchemaID, record.FieldName)
			}
		}

		for _, record := range req.Records {
			grant, err := upsertActiveGrant(tx, req.ApplicationID, record, expiresAt, now)
			if err != nil {
				return err
			}
			records = append(records, grant.ToResponse())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &models.AccessGrantListResponse{Records: records}, nil
}

// upsertActiveGrant renews the application's active grant for the field, or
// creates one when there is none.
func upsertActiveGrant(
	tx *gorm.DB,
	applicationID string,
	record models.AccessGrantCreateRequestRecord,
	expiresAt, now time.Time,
) (*models.AccessGrant, error) {
	// Find rather than Take: no active grant yet is the usual case, not an error.
	var grant models.AccessGrant
	result := tx.
		Where("schema_id = ? AND field_name = ? AND application_id = ? AND status = ?",
			record.SchemaID, record.FieldName, applicationID, models.GrantStatusActive).
		Limit(1).
		Find(&grant)

	switch {
	case result.Error != nil:
		return nil, fmt.Errorf("failed to fetch access grant: %w", result.Error)
	case result.RowsAffected == 0:
		grant = models.AccessGrant{
			ID:            uuid.New(),
			SchemaID:      record.SchemaID,
			FieldName:     record.FieldName,
			ApplicationID: applicationID,
			ExpiresAt:     expiresAt,
			Status:        models.GrantStatusActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&grant).Error; err != nil {
			return nil, fmt.Errorf("failed to create access grant: %w", err)
		}
	default:
		grant.ExpiresAt = expiresAt
		grant.UpdatedAt = now
		if err := tx.Model(&grant).
			Select("expires_at", "updated_at").
			Updates(&grant).Error; err != nil {
			return nil, fmt.Errorf("failed to renew access grant: %w", err)
		}
	}
	return &grant, nil
}

// ListAccessGrants returns the access grants that match the filter.
func (s *PolicyMetadataService) ListAccessGrants(filter models.AccessGrantFilter) (*models.AccessGrantListResponse, error) {
	query := s.db.Model(&models.AccessGrant{})
	if filter.ApplicationID != "" {
		query = query.Where("application_id = ?", filter.ApplicationID)
	}
	if filter.SchemaID != "" {
		query = query.Where("schema_id = ?", filter.SchemaID)
	}
	if filter.Status != "" {
		if !filter.Status.IsValid() {
			return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidGrantRequest, filter.Status)
		}
		query = query.Where("status = ?", filter.Status)
	}

	var grants []models.AccessGrant
	if err := query.
		Order("schema_id ASC, field_name ASC, application_id ASC, created_at ASC").
		Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("failed to list access grants: %w", err)
	}

	records := make([]models.AccessGrantResponse, 0, len(grants))
	for i := range grants {
		records = append(records, grants[i].ToResponse())
	}
	return &models.AccessGrantListResponse{Records: records}, nil
}

// RevokeAccessGrant revokes one active grant. The grant is kept, with status
// revoked, and no longer gives the application access.
func (s *PolicyMetadataService) RevokeAccessGrant(id string) (*models.AccessGrantResponse, error) {
	var grant models.AccessGrant
	err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("CAST(id AS TEXT) = ? AND status = ?", id, models.GrantStatusActive).
			Limit(1).
			Find(&grant)
		if result.Error != nil {
			return fmt.Errorf("failed to fetch access grant: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("%w: %s", ErrAccessGrantNotFound, id)
		}

		grant.Status = models.GrantStatusRevoked
		grant.UpdatedAt = time.Now().UTC()
		if err := tx.Model(&grant).
			Select("status", "updated_at").
			Updates(&grant).Error; err != nil {
			return fmt.Errorf("failed to revoke access grant: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	response := grant.ToResponse()
	return &response, nil
}
