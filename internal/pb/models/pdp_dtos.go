package models

// Request and Response DTOs for Policy Metadata and Allow List Management

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

// AllowListUpdateRequest represents the request to update allow list
type AllowListUpdateRequest struct {
	ApplicationID string                `json:"applicationId" validate:"required"`
	Records       []SelectedFieldRecord `json:"records" validate:"required,dive"`
	GrantDuration GrantDurationType     `json:"grantDuration" validate:"required,grant_duration_type_enum"`
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
