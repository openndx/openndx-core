package decision

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrFieldNotFound reports a requested field that has no policy.
var ErrFieldNotFound = errors.New("policy metadata not found")

// Service decides an application's access to the fields it asks for.
type Service struct {
	fields FieldReader
	grants GrantReader
	now    func() time.Time
}

// NewService creates a decision Service that reads field policies from fields
// and the application's grants from grants.
func NewService(fields FieldReader, grants GrantReader) *Service {
	return &Service{fields: fields, grants: grants, now: time.Now}
}

// GetPolicyDecision decides each requested field and derives the legacy lists
// and flags from the outcomes. A requested field with no policy fails the
// whole request with ErrFieldNotFound.
func (s *Service) GetPolicyDecision(ctx context.Context, req *PolicyDecisionRequest) (*PolicyDecisionResponse, error) {
	keys := make([]FieldKey, 0, len(req.RequiredFields))
	for _, record := range req.RequiredFields {
		keys = append(keys, FieldKey{SchemaID: record.SchemaID, FieldName: record.FieldName})
	}

	policies, err := s.fields.FieldPolicies(ctx, keys)
	if err != nil {
		return nil, err
	}
	policyByKey := make(map[FieldKey]*FieldPolicy, len(policies))
	for i := range policies {
		policyByKey[policies[i].FieldKey] = &policies[i]
	}

	// An expired grant is still passed to Decide, which reports the field as
	// expired.
	grants, err := s.grants.ActiveGrants(ctx, req.ApplicationID, keys)
	if err != nil {
		return nil, err
	}
	now := s.now()
	grantByKey := make(map[FieldKey]*Grant, len(grants))
	for i := range grants {
		key := grants[i].FieldKey
		grantByKey[key] = preferredGrant(grantByKey[key], &grants[i].Grant, now)
	}

	resp := &PolicyDecisionResponse{Fields: make([]PolicyDecisionFieldOutcome, 0, len(keys))}
	for _, key := range keys {
		field, ok := policyByKey[key]
		if !ok {
			return nil, fmt.Errorf("%w for schema_id %s and field_name %s", ErrFieldNotFound, key.SchemaID, key.FieldName)
		}

		result := Decide(field.Policy, req.ApplicationID, grantByKey[key], now)
		resp.Fields = append(resp.Fields, PolicyDecisionFieldOutcome{
			SchemaID:    key.SchemaID,
			FieldName:   key.FieldName,
			Outcome:     string(result.Outcome),
			Reason:      result.Reason,
			Obligations: result.Obligations,
		})

		record := PolicyDecisionResponseFieldRecord{
			FieldName:   key.FieldName,
			SchemaID:    key.SchemaID,
			DisplayName: field.DisplayName,
			Description: field.Description,
			Owner:       field.Owner,
		}
		switch result.Outcome {
		case OutcomeAllow:
		case OutcomeConsentRequired:
			resp.ConsentRequiredFields = append(resp.ConsentRequiredFields, record)
		case OutcomeExpired:
			resp.ExpiredFields = append(resp.ExpiredFields, record)
		default:
			resp.UnauthorizedFields = append(resp.UnauthorizedFields, record)
		}
	}

	resp.AppAuthorized = len(resp.UnauthorizedFields) == 0
	resp.AppAccessExpired = len(resp.ExpiredFields) > 0
	resp.AppRequiresOwnerConsent = len(resp.ConsentRequiredFields) > 0
	return resp, nil
}

// preferredGrant picks the grant to evaluate when an application holds both a
// plain and a consent bypass grant for a field: the bypass grant while it is
// unexpired, otherwise the plain one. current may be nil.
func preferredGrant(current, candidate *Grant, now time.Time) *Grant {
	if current == nil {
		return candidate
	}
	bypass, plain := current, candidate
	if !bypass.ConsentBypass {
		bypass, plain = candidate, current
	}
	if bypass.ConsentBypass && !bypass.Expired(now) {
		return bypass
	}
	return plain
}
