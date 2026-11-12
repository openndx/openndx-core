package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openndx/openndx-core/internal/pdp/decision"
	"github.com/openndx/openndx-core/internal/pdp/fieldpolicy"
	"github.com/openndx/openndx-core/internal/pdp/models"
	"gorm.io/gorm"
)

var (
	ErrAccessGrantNotFound = errors.New("access grant not found")
	ErrInvalidGrantRequest = errors.New("invalid access grant request")
)

// FieldReader reads the policy metadata of the fields a decision is asked about.
type FieldReader interface {
	FieldsByKeys(ctx context.Context, keys []fieldpolicy.FieldKey) ([]fieldpolicy.PolicyMetadata, error)
}

// PolicyMetadataService provides business logic for access grants and policy decisions
type PolicyMetadataService struct {
	db     *gorm.DB
	fields FieldReader
}

// NewPolicyMetadataService creates a new policy metadata service
func NewPolicyMetadataService(db *gorm.DB, fields FieldReader) *PolicyMetadataService {
	return &PolicyMetadataService{db: db, fields: fields}
}

// GetPolicyDecision evaluates the policy for each requested field
func (s *PolicyMetadataService) GetPolicyDecision(ctx context.Context, req *models.PolicyDecisionRequest) (*models.PolicyDecisionResponse, error) {
	// Collect the requested fields and their unique schema IDs
	keys := make([]fieldpolicy.FieldKey, 0, len(req.RequiredFields))
	schemaIDSet := make(map[string]struct{})
	for _, record := range req.RequiredFields {
		keys = append(keys, fieldpolicy.FieldKey{SchemaID: record.SchemaID, FieldName: record.FieldName})
		schemaIDSet[record.SchemaID] = struct{}{}
	}

	var schemaIDs []string
	for schemaID := range schemaIDSet {
		schemaIDs = append(schemaIDs, schemaID)
	}

	allMetadata, err := s.fields.FieldsByKeys(ctx, keys)
	if err != nil {
		return nil, err
	}

	// Create map for fast lookup: (schema_id + field_name) -> &PolicyMetadata
	metadataMap := make(map[string]*fieldpolicy.PolicyMetadata)
	for i := range allMetadata {
		pm := &allMetadata[i]
		key := pm.SchemaID + ":" + pm.FieldName
		metadataMap[key] = pm
	}

	// Load the application's active grants for those schemas. An expired grant
	// is still passed to the decision, which reports the field as expired.
	var grants []models.AccessGrant
	if err := s.db.WithContext(ctx).
		Where("application_id = ? AND status = ? AND schema_id IN ?",
			req.ApplicationID, models.GrantStatusActive, schemaIDs).
		Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch access grants: %w", err)
	}
	now := time.Now()
	grantMap := make(map[string]*models.AccessGrant, len(grants))
	for i := range grants {
		key := grants[i].SchemaID + ":" + grants[i].FieldName
		grantMap[key] = preferredGrant(grantMap[key], &grants[i], now)
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

		var grant *decision.Grant
		if g, ok := grantMap[key]; ok {
			grant = &decision.Grant{ExpiresAt: g.ExpiresAt, ConsentBypass: g.ConsentBypass}
		}
		policy := decision.Field{RequiresAllowlist: pm.RequiresAllowlist, RequiresConsent: pm.RequiresConsent}
		result := decision.Decide(policy, req.ApplicationID, grant, now)

		fields = append(fields, models.PolicyDecisionFieldOutcome{
			SchemaID:    pm.SchemaID,
			FieldName:   pm.FieldName,
			Outcome:     string(result.Outcome),
			Reason:      result.Reason,
			Obligations: result.Obligations,
		})

		fieldRecord := models.PolicyDecisionResponseFieldRecord{
			FieldName:   pm.FieldName,
			SchemaID:    pm.SchemaID,
			DisplayName: pm.DisplayName,
			Description: pm.Description,
			Owner:       pm.Owner,
		}
		switch result.Outcome {
		case decision.OutcomeAllow:
		case decision.OutcomeConsentRequired:
			consentRequiredFields = append(consentRequiredFields, fieldRecord)
		case decision.OutcomeExpired:
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

// preferredGrant picks the grant to evaluate when an application holds both a
// plain and a consent bypass grant for a field: the bypass grant while it is
// unexpired, otherwise the plain one. current may be nil.
func preferredGrant(current, candidate *models.AccessGrant, now time.Time) *models.AccessGrant {
	if current == nil {
		return candidate
	}
	bypass, plain := current, candidate
	if !bypass.ConsentBypass {
		bypass, plain = candidate, current
	}
	if bypass.ConsentBypass && now.Before(bypass.ExpiresAt) {
		return bypass
	}
	return plain
}
