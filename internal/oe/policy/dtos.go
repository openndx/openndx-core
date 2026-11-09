package policy

// RequiredField represents a field that requires policy decision
type RequiredField struct {
	FieldName string `json:"fieldName"`
	SchemaID  string `json:"schemaId"`
}

// PdpRequest represents a policy decision request
type PdpRequest struct {
	AppId          string          `json:"applicationId"`
	RequiredFields []RequiredField `json:"requiredFields"`
}

// ConsentRequiredField represents a field that requires consent
// Matches PolicyDecisionResponseFieldRecord DTO structure from PolicyDecisionPoint
type ConsentRequiredField struct {
	FieldName   string     `json:"fieldName"`
	SchemaID    string     `json:"schemaId"`
	DisplayName *string    `json:"displayName,omitempty"`
	Description *string    `json:"description,omitempty"`
	Owner       *OwnerType `json:"owner,omitempty"`
}

// Field outcomes the PDP returns. The set is fixed.
const (
	OutcomeAllow           = "allow"
	OutcomeDeny            = "deny"
	OutcomeExpired         = "expired"
	OutcomeConsentRequired = "consent_required"
)

// FieldOutcome is the PDP's decision for one requested field. Obligations name
// extra actions the policy asks the Orchestration Engine to take; the audit
// trail records them with the outcome.
// Matches PolicyDecisionFieldOutcome from PolicyDecisionPoint.
type FieldOutcome struct {
	SchemaID    string   `json:"schemaId"`
	FieldName   string   `json:"fieldName"`
	Outcome     string   `json:"outcome"`
	Reason      string   `json:"reason,omitempty"`
	Obligations []string `json:"obligations,omitempty"`
}

// PdpResponse represents a policy decision response. Fields holds the decision
// for every requested field; the lists and flags are derived from it.
type PdpResponse struct {
	Fields                  []FieldOutcome         `json:"fields"`
	AppAuthorized           bool                   `json:"appAuthorized"`
	UnauthorizedFields      []ConsentRequiredField `json:"unauthorizedFields"`
	AppAccessExpired        bool                   `json:"appAccessExpired"`
	ExpiredFields           []ConsentRequiredField `json:"expiredFields"`
	AppRequiresOwnerConsent bool                   `json:"appRequiresOwnerConsent"`
	ConsentRequiredFields   []ConsentRequiredField `json:"consentRequiredFields"`
}
