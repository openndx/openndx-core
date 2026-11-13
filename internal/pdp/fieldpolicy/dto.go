package fieldpolicy

import (
	"bytes"
	"encoding/json"
)

// PolicyCreateRequestRecord is one field's policy in a PolicyCreateRequest.
type PolicyCreateRequestRecord struct {
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

// PolicyCreateRequest replaces the field policies of one schema.
type PolicyCreateRequest struct {
	SchemaID string                      `json:"schemaId" validate:"required"`
	Records  []PolicyCreateRequestRecord `json:"records" validate:"required,dive"`
}

// PolicyResponse is one field policy as the API returns it.
type PolicyResponse struct {
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

// PolicyCreateResponse lists the field policies a PolicyCreateRequest stored.
type PolicyCreateResponse struct {
	Records []PolicyResponse `json:"records"`
}

type PolicyListResponse struct {
	Records []PolicyResponse `json:"records"`
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

// PolicyPatchRequest contains the fields that can be changed
// independently on one field policy.
type PolicyPatchRequest struct {
	DisplayName       OptionalPatchField[string] `json:"displayName"`
	Description       OptionalPatchField[string] `json:"description"`
	RequiresAllowlist OptionalPatchField[bool]   `json:"requiresAllowlist"`
	RequiresConsent   OptionalPatchField[bool]   `json:"requiresConsent"`
}

// IsEmpty reports whether the PATCH request contains no editable properties.
func (r PolicyPatchRequest) IsEmpty() bool {
	return !r.DisplayName.Set &&
		!r.Description.Set &&
		!r.RequiresAllowlist.Set &&
		!r.RequiresConsent.Set
}

// NullFlag returns the name of a policy flag the request sets to null, or "".
// The flags can be changed but not cleared.
func (r PolicyPatchRequest) NullFlag() string {
	switch {
	case r.RequiresAllowlist.Set && r.RequiresAllowlist.Value == nil:
		return "requiresAllowlist"
	case r.RequiresConsent.Set && r.RequiresConsent.Value == nil:
		return "requiresConsent"
	}
	return ""
}
