package decision

// PolicyDecisionRequestRecord names one field the application asks to read.
type PolicyDecisionRequestRecord struct {
	FieldName string `json:"fieldName"`
	SchemaID  string `json:"schemaId"`
}

// PolicyDecisionRequest asks for the decision on each field an application
// wants to read.
type PolicyDecisionRequest struct {
	ApplicationID  string                        `json:"applicationId" validate:"required"`
	RequiredFields []PolicyDecisionRequestRecord `json:"requiredFields" validate:"required,dive"`
}

// PolicyDecisionResponseFieldRecord describes a field in the legacy outcome
// lists of a PolicyDecisionResponse.
type PolicyDecisionResponseFieldRecord struct {
	FieldName   string  `json:"fieldName"`
	SchemaID    string  `json:"schemaId"`
	DisplayName *string `json:"displayName,omitempty"`
	Description *string `json:"description,omitempty"`
	Owner       *string `json:"owner,omitempty"`
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
