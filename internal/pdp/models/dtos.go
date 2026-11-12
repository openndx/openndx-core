package models

import "github.com/openndx/openndx-core/internal/pdp/fieldpolicy"

// Request and Response DTOs

// AccessGrantCreateRequestRecord names one field an application is granted.
type AccessGrantCreateRequestRecord struct {
	FieldName string `json:"fieldName"`
	SchemaID  string `json:"schemaId"`
}

// AccessGrantCreateRequest grants one application access to fields for a
// duration. A field the application already holds an active grant of the same
// kind for has that grant renewed.
//
// With ConsentBypass set, the grants are consent bypass grants: LegalBasis is
// required, and every field must ask for the owner's consent.
type AccessGrantCreateRequest struct {
	ApplicationID string                           `json:"applicationId"`
	Records       []AccessGrantCreateRequestRecord `json:"records"`
	GrantDuration GrantDurationType                `json:"grantDuration"`
	ConsentBypass bool                             `json:"consentBypass,omitempty"`
	LegalBasis    string                           `json:"legalBasis,omitempty"`
}

// AccessGrantResponse is one access grant.
type AccessGrantResponse struct {
	ID            string      `json:"id"`
	SchemaID      string      `json:"schemaId"`
	FieldName     string      `json:"fieldName"`
	ApplicationID string      `json:"applicationId"`
	ExpiresAt     string      `json:"expiresAt"`
	Status        GrantStatus `json:"status"`
	ConsentBypass bool        `json:"consentBypass"`
	LegalBasis    *string     `json:"legalBasis,omitempty"`
	CreatedAt     string      `json:"createdAt"`
	UpdatedAt     string      `json:"updatedAt"`
}

// AccessGrantListResponse holds access grants, for both creating and listing them.
type AccessGrantListResponse struct {
	Records []AccessGrantResponse `json:"records"`
}

// AccessGrantFilter narrows a grant listing. Empty fields match every grant.
type AccessGrantFilter struct {
	ApplicationID string
	SchemaID      string
	Status        GrantStatus
	// ConsentBypass, when set, matches only bypass (true) or plain (false) grants.
	ConsentBypass *bool
}

// PolicyDecisionRequestRecord represents a policy decision request record
type PolicyDecisionRequestRecord struct {
	FieldName string `json:"fieldName"`
	SchemaID  string `json:"schemaId"`
}

// PolicyDecisionRequest represents a policy decision request
type PolicyDecisionRequest struct {
	ApplicationID  string                        `json:"applicationId" validate:"required"`
	RequiredFields []PolicyDecisionRequestRecord `json:"requiredFields" validate:"required,dive"`
}

// PolicyDecisionResponseFieldRecord represents a policy decision response record
type PolicyDecisionResponseFieldRecord struct {
	FieldName   string             `json:"fieldName"`
	SchemaID    string             `json:"schemaId"`
	DisplayName *string            `json:"displayName,omitempty"`
	Description *string            `json:"description,omitempty"`
	Owner       *fieldpolicy.Owner `json:"owner,omitempty"`
}

// PolicyDecisionFieldOutcome is the decision for one requested field.
type PolicyDecisionFieldOutcome struct {
	SchemaID    string   `json:"schemaId"`
	FieldName   string   `json:"fieldName"`
	Outcome     string   `json:"outcome"`
	Reason      string   `json:"reason,omitempty"`
	Obligations []string `json:"obligations,omitempty"`
}

// PolicyDecisionResponse represents a policy decision response. Fields holds
// the decision for every requested field, in request order; the lists and
// flags after it are derived from Fields.
type PolicyDecisionResponse struct {
	Fields                  []PolicyDecisionFieldOutcome        `json:"fields"`
	AppAuthorized           bool                                `json:"appAuthorized"`
	UnauthorizedFields      []PolicyDecisionResponseFieldRecord `json:"unauthorizedFields"`
	AppAccessExpired        bool                                `json:"appAccessExpired"`
	ExpiredFields           []PolicyDecisionResponseFieldRecord `json:"expiredFields"`
	AppRequiresOwnerConsent bool                                `json:"appRequiresOwnerConsent"`
	ConsentRequiredFields   []PolicyDecisionResponseFieldRecord `json:"consentRequiredFields"`
}
