package schema

import "github.com/openndx/openndx-core/internal/pb/policy"

// CreateSchemaSubmissionRequest Provider Schema Submission DTOs
type CreateSchemaSubmissionRequest struct {
	SchemaName        string  `json:"schemaName" validate:"required"`
	SchemaDescription *string `json:"schemaDescription,omitempty"`
	SDL               string  `json:"sdl" validate:"required"`
	SchemaEndpoint    string  `json:"schemaEndpoint" validate:"required"`
	PreviousSchemaID  *string `json:"previousSchemaId,omitempty"`
	MemberID          string  `json:"memberId" validate:"required"`
}

// UpdateSchemaSubmissionRequest updates the status of a provider schema submission
type UpdateSchemaSubmissionRequest struct {
	SchemaName        *string `json:"schemaName,omitempty"`
	SchemaDescription *string `json:"schemaDescription,omitempty"`
	SDL               *string `json:"sdl,omitempty"`
	SchemaEndpoint    *string `json:"schemaEndpoint,omitempty"`
	Status            *string `json:"status,omitempty"`
	PreviousSchemaID  *string `json:"previousSchemaId,omitempty"`
	Review            *string `json:"review,omitempty"`
}

// CreateSchemaRequest creates a new provider schema. Exactly one of SDL or
// Fields must be provided.
type CreateSchemaRequest struct {
	// SchemaID optionally fixes the schema's ID, e.g. "drp-person-v1", so the
	// Orchestration Engine's SDL and config can reference it. A UUID is
	// generated when it's empty. It can't be changed after creation.
	SchemaID          string  `json:"schemaId,omitempty"`
	SchemaName        string  `json:"schemaName" validate:"required"`
	SchemaDescription *string `json:"schemaDescription,omitempty"`
	// SDL is a GraphQL SDL string annotated with @accessControl(allowlist: ..., consent: ...)
	// and @source (and optionally @displayName/@description/@owner) directives,
	// parsed into policy metadata records. Mutually exclusive with Fields.
	SDL string `json:"sdl,omitempty"`
	// Fields lets a caller declare policy metadata records directly, skipping
	// SDL/GraphQL parsing entirely - each FieldName is used as-is (no forced
	// typename.fieldName prefix the way SDL-derived field paths get).
	// Mutually exclusive with SDL.
	Fields   []policy.PolicyMetadataCreateRequestRecord `json:"fields,omitempty"`
	Endpoint string                                     `json:"endpoint" validate:"required"`
	MemberID string                                     `json:"memberId" validate:"required"`
}

// UpdateSchemaRequest updates an existing provider schema
type UpdateSchemaRequest struct {
	SchemaName        *string `json:"schemaName,omitempty"`
	SchemaDescription *string `json:"schemaDescription,omitempty"`
	SDL               *string `json:"sdl,omitempty"`
	Endpoint          *string `json:"endpoint,omitempty"`
	Version           *string `json:"version,omitempty"`
}

type SchemaResponse struct {
	SchemaID          string  `json:"schemaId"`
	MemberID          string  `json:"memberId"`
	SchemaName        string  `json:"schemaName"`
	SDL               string  `json:"sdl"`
	Endpoint          string  `json:"endpoint"`
	Version           string  `json:"version"`
	SchemaDescription *string `json:"schemaDescription,omitempty"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
}

type SchemaSubmissionResponse struct {
	SubmissionID      string  `json:"submissionId"`
	PreviousSchemaID  *string `json:"previousSchemaId,omitempty"`
	SchemaName        string  `json:"schemaName"`
	SchemaDescription *string `json:"schemaDescription,omitempty"`
	SDL               string  `json:"sdl"`
	SchemaEndpoint    string  `json:"schemaEndpoint"`
	Status            string  `json:"status"`
	MemberID          string  `json:"memberId"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	Review            *string `json:"review,omitempty"`
}
