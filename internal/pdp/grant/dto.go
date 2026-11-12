package grant

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
