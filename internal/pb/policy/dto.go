package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// Request and Response DTOs for Policy Metadata and Allow List Management

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
	AllowList    AllowList    `json:"allowList"`
	Owner        *Owner       `json:"owner,omitempty"`
	CreatedAt    string       `json:"createdAt"`
	UpdatedAt    string       `json:"updatedAt"`
}

// PolicyMetadataCreateResponse represents the response from policy metadata creation
type PolicyMetadataCreateResponse struct {
	Records []PolicyMetadataResponse `json:"records"`
}

// PolicyMetadataListResponse represents the policy metadata records of a schema
type PolicyMetadataListResponse struct {
	Records []PolicyMetadataResponse `json:"records"`
}

// OptionalPatchField distinguishes between an omitted JSON property and a
// property explicitly set to null, so a PATCH body can be forwarded to the
// PDP without changing its meaning.
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

// MarshalJSON encodes the field as its value, or null when it was set to null.
// Omitted fields are dropped by the omitzero tag on the containing struct.
func (o OptionalPatchField[T]) MarshalJSON() ([]byte, error) {
	if o.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*o.Value)
}

// PolicyMetadataPatchRequest contains the fields that can be changed
// independently on one policy metadata record.
type PolicyMetadataPatchRequest struct {
	DisplayName  OptionalPatchField[string]       `json:"displayName,omitzero"`
	Description  OptionalPatchField[string]       `json:"description,omitzero"`
	PolicyClass  OptionalPatchField[string]       `json:"policyClass,omitzero"`
	PolicyParams OptionalPatchField[PolicyParams] `json:"policyParams,omitzero"`
}

// IsEmpty reports whether the PATCH request contains no editable properties.
func (r PolicyMetadataPatchRequest) IsEmpty() bool {
	return !r.DisplayName.Set &&
		!r.Description.Set &&
		!r.PolicyClass.Set &&
		!r.PolicyParams.Set
}

// AccessGrantCreateRequest grants one application access to fields for a
// duration. The PDP renews a field's existing active grant.
type AccessGrantCreateRequest struct {
	ApplicationID string                `json:"applicationId" validate:"required"`
	Records       []SelectedFieldRecord `json:"records" validate:"required,dive"`
	GrantDuration GrantDurationType     `json:"grantDuration" validate:"required,grant_duration_type_enum"`
}

// Access grant statuses
const (
	GrantStatusActive  = "active"
	GrantStatusRevoked = "revoked"
)

// AccessGrant is one application's access to one field in the PDP.
type AccessGrant struct {
	ID            string `json:"id"`
	SchemaID      string `json:"schemaId"`
	FieldName     string `json:"fieldName"`
	ApplicationID string `json:"applicationId"`
	ExpiresAt     string `json:"expiresAt"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// AllowListEntry returns the grant as an allow-list entry.
func (g AccessGrant) AllowListEntry() (AllowListEntry, error) {
	expiresAt, err := time.Parse(time.RFC3339, g.ExpiresAt)
	if err != nil {
		return AllowListEntry{}, fmt.Errorf("access grant %s: invalid expiresAt: %w", g.ID, err)
	}
	updatedAt, err := time.Parse(time.RFC3339, g.UpdatedAt)
	if err != nil {
		return AllowListEntry{}, fmt.Errorf("access grant %s: invalid updatedAt: %w", g.ID, err)
	}
	return AllowListEntry{ExpiresAt: expiresAt, UpdatedAt: updatedAt}, nil
}

// AccessGrantListResponse holds access grants, for both creating and listing them.
type AccessGrantListResponse struct {
	Records []AccessGrant `json:"records"`
}

// AccessGrantFilter narrows a grant listing. Empty fields match every grant.
type AccessGrantFilter struct {
	ApplicationID string
	SchemaID      string
	Status        string
}
