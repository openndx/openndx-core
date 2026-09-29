package models

import (
	"bytes"
	"encoding/json"
)

// Request and Response DTOs

// PolicyMetadataCreateRequestRecord represents the request to create policy metadata
type PolicyMetadataCreateRequestRecord struct {
	FieldName         string            `json:"fieldName" validate:"required"`
	DisplayName       *string           `json:"displayName,omitempty"`
	Description       *string           `json:"description,omitempty"`
	Source            Source            `json:"source" validate:"required,source_enum"`
	IsOwner           bool              `json:"isOwner" validate:"required"`
	AccessControlType AccessControlType `json:"accessControlType" validate:"required,access_control_type_enum"`
	Owner             *Owner            `json:"owner,omitempty" validate:"omitempty,owner_enum"`
}

// PolicyMetadataCreateRequest represents the request to create policy metadata
type PolicyMetadataCreateRequest struct {
	SchemaID string                              `json:"schemaId" validate:"required"`
	Records  []PolicyMetadataCreateRequestRecord `json:"records" validate:"required,dive"`
}

// PolicyMetadataResponse represents the response from policy metadata operations
type PolicyMetadataResponse struct {
	ID                string            `json:"id"`
	SchemaID          string            `json:"schemaId"`
	FieldName         string            `json:"fieldName"`
	DisplayName       *string           `json:"displayName,omitempty"`
	Description       *string           `json:"description,omitempty"`
	Source            Source            `json:"source"`
	IsOwner           bool              `json:"isOwner"`
	AccessControlType AccessControlType `json:"accessControlType"`
	AllowList         AllowList         `json:"allowList"`
	Owner             *Owner            `json:"owner,omitempty"`
	CreatedAt         string            `json:"createdAt"`
	UpdatedAt         string            `json:"updatedAt"`
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
	DisplayName       OptionalPatchField[string]            `json:"displayName"`
	Description       OptionalPatchField[string]            `json:"description"`
	AccessControlType OptionalPatchField[AccessControlType] `json:"accessControlType"`
}

// IsEmpty reports whether the PATCH request contains no editable properties.
func (r PolicyMetadataPatchRequest) IsEmpty() bool {
	return !r.DisplayName.Set &&
		!r.Description.Set &&
		!r.AccessControlType.Set
}

// AllowListUpdateRequestRecord represents the one record of request to update allow list
type AllowListUpdateRequestRecord struct {
	FieldName string `json:"fieldName" validate:"required"`
	SchemaID  string `json:"schemaId" validate:"required"`
}

// AllowListUpdateRequest represents the request to update allow list
type AllowListUpdateRequest struct {
	ApplicationID string                         `json:"applicationId" validate:"required"`
	Records       []AllowListUpdateRequestRecord `json:"records" validate:"required,dive"`
	GrantDuration GrantDurationType              `json:"grantDuration" validate:"required,grant_duration_type_enum"`
}

// AllowListUpdateResponseRecord represents one record in the allow list update response
type AllowListUpdateResponseRecord struct {
	FieldName string `json:"fieldName"`
	SchemaID  string `json:"schemaId"`
	ExpiresAt string `json:"expiresAt"`
	UpdatedAt string `json:"updatedAt"`
}

// AllowListUpdateResponse represents the response from allow list update
type AllowListUpdateResponse struct {
	Records []AllowListUpdateResponseRecord `json:"records"`
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

// PolicyDecisionResponse represents a policy decision response
type PolicyDecisionResponse struct {
	AppAuthorized           bool                                `json:"appAuthorized"`
	UnauthorizedFields      []PolicyDecisionResponseFieldRecord `json:"unauthorizedFields"`
	AppAccessExpired        bool                                `json:"appAccessExpired"`
	ExpiredFields           []PolicyDecisionResponseFieldRecord `json:"expiredFields"`
	AppRequiresOwnerConsent bool                                `json:"appRequiresOwnerConsent"`
	ConsentRequiredFields   []PolicyDecisionResponseFieldRecord `json:"consentRequiredFields"`
}
