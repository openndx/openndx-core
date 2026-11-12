package grant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAccessGrantNotFound = errors.New("access grant not found")
	ErrInvalidGrantRequest = errors.New("invalid access grant request")
	// ErrFieldNotFound reports a grant for a field that has no policy
	// metadata. Its text matches the field policy context's not found error,
	// so API clients see the same message as before grants had their own
	// package.
	ErrFieldNotFound = errors.New("policy metadata not found")
)

// Service manages applications' access grants to fields.
type Service struct {
	db *gorm.DB
}

// NewService creates a new access grant service
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// CreateAccessGrants grants an application access to fields for the requested
// duration. A field the application already holds an active grant of the same
// kind for has that grant's expiry renewed, so renewing plain grants never
// touches a consent bypass grant and the other way round. Every field must
// have policy metadata.
//
// With req.ConsentBypass set, the grants are consent bypass grants. They need
// a legal basis, which a renewal replaces, and every field must ask for the
// owner's consent: fields that don't require consent are rejected.
func (s *Service) CreateAccessGrants(req *AccessGrantCreateRequest) (*AccessGrantListResponse, error) {
	if strings.TrimSpace(req.ApplicationID) == "" {
		return nil, fmt.Errorf("%w: applicationId is required", ErrInvalidGrantRequest)
	}
	for _, record := range req.Records {
		if record.SchemaID == "" || record.FieldName == "" {
			return nil, fmt.Errorf("%w: every record needs a schemaId and a fieldName", ErrInvalidGrantRequest)
		}
	}
	var legalBasis *string
	if basis := strings.TrimSpace(req.LegalBasis); req.ConsentBypass {
		if basis == "" {
			return nil, fmt.Errorf("%w: legalBasis is required for consent bypass grants", ErrInvalidGrantRequest)
		}
		legalBasis = &basis
	} else if basis != "" {
		return nil, fmt.Errorf("%w: legalBasis applies to consent bypass grants only", ErrInvalidGrantRequest)
	}

	now := time.Now().UTC()
	expiresAt, ok := req.GrantDuration.ExpiresAt(now)
	if !ok {
		return nil, fmt.Errorf("%w: invalid grant duration %q", ErrInvalidGrantRequest, req.GrantDuration)
	}

	records := make([]AccessGrantResponse, 0, len(req.Records))
	if len(req.Records) == 0 {
		return &AccessGrantListResponse{Records: records}, nil
	}

	pairs := make([][]any, 0, len(req.Records))
	for _, record := range req.Records {
		pairs = append(pairs, []any{record.SchemaID, record.FieldName})
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Lock the fields' policy metadata so concurrent grants for the same
		// field are serialized and cannot both create an active grant.
		var fields []field
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("(schema_id, field_name) IN ?", pairs).
			Order("schema_id ASC, field_name ASC").
			Find(&fields).Error; err != nil {
			return fmt.Errorf("failed to fetch policy metadata records: %w", err)
		}

		requiresConsent := make(map[FieldKey]bool, len(fields))
		for _, f := range fields {
			requiresConsent[FieldKey{SchemaID: f.SchemaID, FieldName: f.FieldName}] = f.RequiresConsent
		}
		for _, record := range req.Records {
			consent, ok := requiresConsent[FieldKey{SchemaID: record.SchemaID, FieldName: record.FieldName}]
			if !ok {
				return fmt.Errorf("%w for schema_id %s and field_name %s",
					ErrFieldNotFound, record.SchemaID, record.FieldName)
			}
			if req.ConsentBypass && !consent {
				return fmt.Errorf("%w: field %s of schema %s does not require consent, so there is no consent to bypass",
					ErrInvalidGrantRequest, record.FieldName, record.SchemaID)
			}
		}

		for _, record := range req.Records {
			grant, err := upsertActiveGrant(tx, req.ApplicationID, record, legalBasis, expiresAt, now)
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

	return &AccessGrantListResponse{Records: records}, nil
}

// upsertActiveGrant renews the application's active grant for the field, or
// creates one when there is none. A non-nil legalBasis makes it a consent
// bypass grant.
func upsertActiveGrant(
	tx *gorm.DB,
	applicationID string,
	record AccessGrantCreateRequestRecord,
	legalBasis *string,
	expiresAt, now time.Time,
) (*AccessGrant, error) {
	consentBypass := legalBasis != nil

	// Find rather than Take: no active grant yet is the usual case, not an error.
	var grant AccessGrant
	result := tx.
		Where("schema_id = ? AND field_name = ? AND application_id = ? AND status = ? AND consent_bypass = ?",
			record.SchemaID, record.FieldName, applicationID, GrantStatusActive, consentBypass).
		Limit(1).
		Find(&grant)

	switch {
	case result.Error != nil:
		return nil, fmt.Errorf("failed to fetch access grant: %w", result.Error)
	case result.RowsAffected == 0:
		grant = AccessGrant{
			ID:            uuid.New(),
			SchemaID:      record.SchemaID,
			FieldName:     record.FieldName,
			ApplicationID: applicationID,
			ExpiresAt:     expiresAt,
			Status:        GrantStatusActive,
			ConsentBypass: consentBypass,
			LegalBasis:    legalBasis,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&grant).Error; err != nil {
			return nil, fmt.Errorf("failed to create access grant: %w", err)
		}
	default:
		grant.ExpiresAt = expiresAt
		grant.LegalBasis = legalBasis
		grant.UpdatedAt = now
		if err := tx.Model(&grant).
			Select("expires_at", "legal_basis", "updated_at").
			Updates(&grant).Error; err != nil {
			return nil, fmt.Errorf("failed to renew access grant: %w", err)
		}
	}
	return &grant, nil
}

// ListAccessGrants returns the access grants that match the filter.
func (s *Service) ListAccessGrants(filter AccessGrantFilter) (*AccessGrantListResponse, error) {
	query := s.db.Model(&AccessGrant{})
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
	if filter.ConsentBypass != nil {
		query = query.Where("consent_bypass = ?", *filter.ConsentBypass)
	}

	var grants []AccessGrant
	if err := query.
		Order("schema_id ASC, field_name ASC, application_id ASC, created_at ASC").
		Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("failed to list access grants: %w", err)
	}

	records := make([]AccessGrantResponse, 0, len(grants))
	for i := range grants {
		records = append(records, grants[i].ToResponse())
	}
	return &AccessGrantListResponse{Records: records}, nil
}

// GrantsFor returns the application's active grants for the named fields, in
// no particular order. An active grant that has expired is included: the
// decision reports the field as expired. A field can have both a plain and a
// consent bypass grant.
func (s *Service) GrantsFor(ctx context.Context, applicationID string, keys []FieldKey) ([]AccessGrant, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	pairs := make([][]any, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, []any{key.SchemaID, key.FieldName})
	}

	var grants []AccessGrant
	if err := s.db.WithContext(ctx).
		Where("application_id = ? AND status = ? AND (schema_id, field_name) IN ?",
			applicationID, GrantStatusActive, pairs).
		Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch access grants: %w", err)
	}
	return grants, nil
}

// parseAccessGrantID parses a grant ID. An ID that is not a UUID can't name a
// grant, so it is reported as not found. Querying the uuid column directly,
// not CAST(id AS TEXT), lets PostgreSQL use the primary key index.
func parseAccessGrantID(id string) (uuid.UUID, error) {
	grantID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %s", ErrAccessGrantNotFound, id)
	}
	return grantID, nil
}

// GetAccessGrant returns one access grant.
func (s *Service) GetAccessGrant(id string) (*AccessGrantResponse, error) {
	grantID, err := parseAccessGrantID(id)
	if err != nil {
		return nil, err
	}
	var grant AccessGrant
	result := s.db.Where("id = ?", grantID).Limit(1).Find(&grant)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to fetch access grant: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("%w: %s", ErrAccessGrantNotFound, id)
	}

	response := grant.ToResponse()
	return &response, nil
}

// RevokeAccessGrant revokes one active grant. The grant is kept, with status
// revoked, and no longer gives the application access.
func (s *Service) RevokeAccessGrant(id string) (*AccessGrantResponse, error) {
	grantID, err := parseAccessGrantID(id)
	if err != nil {
		return nil, err
	}
	var grant AccessGrant
	err = s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", grantID, GrantStatusActive).
			Limit(1).
			Find(&grant)
		if result.Error != nil {
			return fmt.Errorf("failed to fetch access grant: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("%w: %s", ErrAccessGrantNotFound, id)
		}

		grant.Status = GrantStatusRevoked
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
