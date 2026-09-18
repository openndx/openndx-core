package models

// Request/Response DTOs for V1 API endpoints

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
	SchemaName        string  `json:"schemaName" validate:"required"`
	SchemaDescription *string `json:"schemaDescription,omitempty"`
	// SDL is a GraphQL SDL string annotated with @accessControl/@source (and
	// optionally @displayName/@description/@isOwner/@owner) directives, parsed
	// into policy metadata records. Mutually exclusive with Fields.
	SDL string `json:"sdl,omitempty"`
	// Fields lets a caller declare policy metadata records directly, skipping
	// SDL/GraphQL parsing entirely - each FieldName is used as-is (no forced
	// typename.fieldName prefix the way SDL-derived field paths get).
	// Mutually exclusive with SDL.
	Fields   []PolicyMetadataCreateRequestRecord `json:"fields,omitempty"`
	Endpoint string                              `json:"endpoint" validate:"required"`
	MemberID string                              `json:"memberId" validate:"required"`
}

// UpdateSchemaRequest updates an existing provider schema
type UpdateSchemaRequest struct {
	SchemaName        *string `json:"schemaName,omitempty"`
	SchemaDescription *string `json:"schemaDescription,omitempty"`
	SDL               *string `json:"sdl,omitempty"`
	Endpoint          *string `json:"endpoint,omitempty"`
	Version           *string `json:"version,omitempty"`
}

// CreateApplicationSubmissionRequest Consumer Application Submission DTOs
type CreateApplicationSubmissionRequest struct {
	ApplicationName        string                `json:"applicationName" validate:"required"`
	ApplicationDescription *string               `json:"applicationDescription,omitempty"`
	SelectedFields         []SelectedFieldRecord `json:"selectedFields" validate:"required,min=1"`
	PreviousApplicationID  *string               `json:"previousApplicationId,omitempty"`
	MemberID               string                `json:"memberId" validate:"required"`
}

// UpdateApplicationSubmissionRequest updates the status of a consumer application submission
type UpdateApplicationSubmissionRequest struct {
	ApplicationName        *string                `json:"applicationName,omitempty"`
	ApplicationDescription *string                `json:"applicationDescription,omitempty"`
	SelectedFields         *[]SelectedFieldRecord `json:"selectedFields,omitempty"`
	Status                 *string                `json:"status,omitempty"`
	PreviousApplicationID  *string                `json:"previousApplicationId,omitempty"`
	Review                 *string                `json:"review,omitempty"`
}

// CreateApplicationRequest creates a new consumer application
type CreateApplicationRequest struct {
	ApplicationName        string                `json:"applicationName" validate:"required"`
	ApplicationDescription *string               `json:"applicationDescription,omitempty"`
	SelectedFields         []SelectedFieldRecord `json:"selectedFields" validate:"required,min=1"`
	MemberID               string                `json:"memberId" validate:"required"`
	// IdpApplicationID and IdpClientID let a caller register an application whose
	// OAuth2 client was already provisioned directly in the IDP (e.g. manually via
	// ThunderID's console, since idpfactory only implements Asgardeo's admin API
	// today - see internal/pb/idp/idpfactory). When both are set, creation skips
	// calling the IDP entirely and uses these values as-is. Must be provided
	// together (both or neither) - a single field alone is rejected.
	IdpApplicationID *string `json:"idpApplicationId,omitempty"`
	IdpClientID      *string `json:"idpClientId,omitempty"`
}

// UpdateApplicationRequest updates an existing consumer application
type UpdateApplicationRequest struct {
	ApplicationName        *string `json:"applicationName,omitempty"`
	ApplicationDescription *string `json:"applicationDescription,omitempty"`
	Version                *string `json:"version,omitempty"`
	// Note: SelectedFields is intentionally omitted from UpdateApplicationRequest.
	// Field/policy updates go through UpdateApplicationPolicyRequest instead.
}

// UpdateApplicationPolicyRequest replaces an existing application's allow-list (its
// requested schema fields and their PDP grant duration).
type UpdateApplicationPolicyRequest struct {
	SelectedFields []SelectedFieldRecord `json:"selectedFields" validate:"required,min=1"`
	GrantDuration  *GrantDurationType    `json:"grantDuration,omitempty" validate:"omitempty,grant_duration_type_enum"`
}

type CreateMemberRequest struct {
	Name        string `json:"name" validate:"required"`
	Email       string `json:"email" validate:"required,email"`
	PhoneNumber string `json:"phoneNumber" validate:"required"`
	// IdpUserID lets a caller register a member whose IDP account was already
	// provisioned directly (e.g. manually via ThunderID's console, since
	// idpfactory only implements Asgardeo's admin API today - see
	// internal/pb/idp/idpfactory). When set, creation skips creating the user
	// and assigning them to the member group in the IDP entirely, and uses
	// this value as-is - the caller is responsible for that group/role
	// assignment having already happened.
	IdpUserID *string `json:"idpUserId,omitempty"`
}

type UpdateMemberRequest struct {
	Name        *string `json:"name,omitempty"`
	PhoneNumber *string `json:"phoneNumber,omitempty"`
}

type MemberResponse struct {
	MemberID    string `json:"memberId"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	IdpUserID   string `json:"idpUserId"`
}

// ToMember converts a MemberResponse to a Member model (for internal use)
func (e *MemberResponse) ToMember() Member {
	return Member{
		MemberID:    e.MemberID,
		Name:        e.Name,
		Email:       e.Email,
		PhoneNumber: e.PhoneNumber,
		IdpUserID:   e.IdpUserID,
	}
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

type ApplicationResponse struct {
	ApplicationID          string                `json:"applicationId"`
	ApplicationName        string                `json:"applicationName"`
	ApplicationDescription *string               `json:"applicationDescription,omitempty"`
	SelectedFields         []SelectedFieldRecord `json:"selectedFields"`
	MemberID               string                `json:"memberId"`
	Version                string                `json:"version"`
	IdpApplicationID       *string               `json:"idpApplicationId,omitempty"`
	IdpClientID            *string               `json:"idpClientId,omitempty"`
	CreatedAt              string                `json:"createdAt"`
	UpdatedAt              string                `json:"updatedAt"`
}

type ApplicationIDResponse struct {
	ApplicationID string `json:"applicationId"`
}

type ApplicationSubmissionResponse struct {
	SubmissionID           string                `json:"submissionId"`
	PreviousApplicationID  *string               `json:"previousApplicationId,omitempty"`
	ApplicationName        string                `json:"applicationName"`
	ApplicationDescription *string               `json:"applicationDescription,omitempty"`
	SelectedFields         []SelectedFieldRecord `json:"selectedFields"`
	MemberID               string                `json:"memberId"`
	Status                 string                `json:"status"`
	CreatedAt              string                `json:"createdAt"`
	UpdatedAt              string                `json:"updatedAt"`
	Review                 *string               `json:"review,omitempty"`
}

// CollectionResponse Generic collection response
type CollectionResponse struct {
	Items interface{} `json:"items"`
	Count int         `json:"count"`
}
