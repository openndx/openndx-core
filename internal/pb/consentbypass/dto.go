package consentbypass

import "github.com/openndx/openndx-core/internal/pb/policy"

// GrantRequest grants an application consent bypass for fields: the
// application reads them without the owner's consent.
type GrantRequest struct {
	Fields        []policy.SelectedFieldRecord `json:"fields" validate:"required,min=1"`
	LegalBasis    string                       `json:"legalBasis" validate:"required"`
	GrantDuration policy.GrantDurationType     `json:"grantDuration" validate:"required,grant_duration_type_enum"`
}

// Grant is one consent bypass grant.
type Grant struct {
	ID string `json:"id"`
	// ApplicationID is the PB application, or "" when the grant's IdP client
	// belongs to no PB application.
	ApplicationID string  `json:"applicationId"`
	IdpClientID   string  `json:"idpClientId"`
	SchemaID      string  `json:"schemaId"`
	FieldName     string  `json:"fieldName"`
	Status        string  `json:"status"`
	LegalBasis    *string `json:"legalBasis,omitempty"`
	ExpiresAt     string  `json:"expiresAt"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}
