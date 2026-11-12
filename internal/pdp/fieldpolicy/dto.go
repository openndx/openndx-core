package fieldpolicy

import (
	"bytes"
	"encoding/json"
)

// PolicyMetadataCreateRequestRecord represents the request to create policy metadata
type PolicyMetadataCreateRequestRecord struct {
	FieldName   string  `json:"fieldName" validate:"required"`
	DisplayName *string `json:"displayName,omitempty"`
	Description *string `json:"description,omitempty"`
	Source      Source  `json:"source" validate:"required,source_enum"`
	// The flags are pointers so that a missing flag is rejected rather than
	// read as false, which would open the field.
	RequiresAllowlist *bool  `json:"requiresAllowlist" validate:"required"`
	RequiresConsent   *bool  `json:"requiresConsent" validate:"required"`
	Owner             *Owner `json:"owner,omitempty" validate:"omitempty,owner_enum"`
}

// PolicyMetadataCreateRequest represents the request to create policy metadata
type PolicyMetadataCreateRequest struct {
	SchemaID string                              `json:"schemaId" validate:"required"`
	Records  []PolicyMetadataCreateRequestRecord `json:"records" validate:"required,dive"`
}

// PolicyMetadataResponse represents the response from policy metadata operations
type PolicyMetadataResponse struct {
	ID                string  `json:"id"`
	SchemaID          string  `json:"schemaId"`
	FieldName         string  `json:"fieldName"`
	DisplayName       *string `json:"displayName,omitempty"`
	Description       *string `json:"description,omitempty"`
	Source            Source  `json:"source"`
	RequiresAllowlist bool    `json:"requiresAllowlist"`
	RequiresConsent   bool    `json:"requiresConsent"`
	Owner             *Owner  `json:"owner,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
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
	DisplayName       OptionalPatchField[string] `json:"displayName"`
	Description       OptionalPatchField[string] `json:"description"`
	RequiresAllowlist OptionalPatchField[bool]   `json:"requiresAllowlist"`
	RequiresConsent   OptionalPatchField[bool]   `json:"requiresConsent"`
}

// IsEmpty reports whether the PATCH request contains no editable properties.
func (r PolicyMetadataPatchRequest) IsEmpty() bool {
	return !r.DisplayName.Set &&
		!r.Description.Set &&
		!r.RequiresAllowlist.Set &&
		!r.RequiresConsent.Set
}

// NullFlag returns the name of a policy flag the request sets to null, or "".
// The flags can be changed but not cleared.
func (r PolicyMetadataPatchRequest) NullFlag() string {
	switch {
	case r.RequiresAllowlist.Set && r.RequiresAllowlist.Value == nil:
		return "requiresAllowlist"
	case r.RequiresConsent.Set && r.RequiresConsent.Value == nil:
		return "requiresConsent"
	}
	return ""
}
