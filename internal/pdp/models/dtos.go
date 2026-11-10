package models

import (
	"bytes"
	"encoding/json"
)

// Request and Response DTOs

// PolicyMetadataCreateRequestRecord represents the request to create policy metadata
type PolicyMetadataCreateRequestRecord struct {
	FieldName    string       `json:"fieldName" validate:"required"`
	DisplayName  *string      `json:"displayName,omitempty"`
	Description  *string      `json:"description,omitempty"`
	Source       Source       `json:"source" validate:"required,source_enum"`
	PolicyClass  string       `json:"policyClass" validate:"required"`
	PolicyParams PolicyParams `json:"policyParams,omitempty"`
	Owner        *Owner       `json:"owner,omitempty" validate:"omitempty,owner_enum"`
}

// PolicyMetadataCreateRequest represents the request to create policy metadata
type PolicyMetadataCreateRequest struct {
	SchemaID string                              `json:"schemaId" validate:"required"`
	Records  []PolicyMetadataCreateRequestRecord `json:"records" validate:"required,dive"`
}

// PolicyMetadataResponse represents the response from policy metadata operations
type PolicyMetadataResponse struct {
	ID           string       `json:"id"`
	SchemaID     string       `json:"schemaId"`
	FieldName    string       `json:"fieldName"`
	DisplayName  *string      `json:"displayName,omitempty"`
	Description  *string      `json:"description,omitempty"`
	Source       Source       `json:"source"`
	PolicyClass  string       `json:"policyClass"`
	PolicyParams PolicyParams `json:"policyParams"`
	Owner        *Owner       `json:"owner,omitempty"`
	CreatedAt    string       `json:"createdAt"`
	UpdatedAt    string       `json:"updatedAt"`
}

// PolicyMetadataCreateResponse represents the response from policy metadata creation
type PolicyMetadataCreateResponse struct {
	Records []PolicyMetadataResponse `json:"records"`
}

type PolicyMetadataListResponse struct {
	Records []PolicyMetadataResponse `json:"records"`
}

// OptionalPatchField distinguishes between an omitted JSON property and a
// property explicitly set to null.
//
// Examples:
// {}                         -> Set is false
// {"displayName": null}      -> Set is true, Value is nil
// {"displayName": "Email"}   -> Set is true, Value points to "Email"
type OptionalPatchField[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON implements custom PATCH field decoding.
func (o *OptionalPatchField[T]) UnmarshalJSON(data []byte) error {
	o.Set = true

	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		o.Value = nil
		return nil
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	o.Value = &value
	return nil
}

// PolicyMetadataPatchRequest contains the fields that can be changed
// independently on one policy metadata record.
type PolicyMetadataPatchRequest struct {
	DisplayName  OptionalPatchField[string]       `json:"displayName"`
	Description  OptionalPatchField[string]       `json:"description"`
	PolicyClass  OptionalPatchField[string]       `json:"policyClass"`
	PolicyParams OptionalPatchField[PolicyParams] `json:"policyParams"`
}

// IsEmpty reports whether the PATCH request contains no editable properties.
func (r PolicyMetadataPatchRequest) IsEmpty() bool {
	return !r.DisplayName.Set &&
		!r.Description.Set &&
		!r.PolicyClass.Set &&
		!r.PolicyParams.Set
}

// AccessGrantCreateRequestRecord names one field an application is granted.
type AccessGrantCreateRequestRecord struct {
	FieldName string `json:"fieldName"`
	SchemaID  string `json:"schemaId"`
}

// AccessGrantCreateRequest grants one application access to fields for a
// duration. A field the application already holds an active grant for has
// that grant renewed.
type AccessGrantCreateRequest struct {
	ApplicationID string                           `json:"applicationId"`
	Records       []AccessGrantCreateRequestRecord `json:"records"`
	GrantDuration GrantDurationType                `json:"grantDuration"`
}

// AccessGrantResponse is one access grant.
type AccessGrantResponse struct {
	ID            string      `json:"id"`
	SchemaID      string      `json:"schemaId"`
	FieldName     string      `json:"fieldName"`
	ApplicationID string      `json:"applicationId"`
	ExpiresAt     string      `json:"expiresAt"`
	Status        GrantStatus `json:"status"`
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
	FieldName   string  `json:"fieldName"`
	SchemaID    string  `json:"schemaId"`
	DisplayName *string `json:"displayName,omitempty"`
	Description *string `json:"description,omitempty"`
	Owner       *Owner  `json:"owner,omitempty"`
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
