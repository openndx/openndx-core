package models

import "github.com/openndx/openndx-core/internal/pdp/fieldpolicy"

// Request and Response DTOs

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
