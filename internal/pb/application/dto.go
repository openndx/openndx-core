package application

import "github.com/openndx/openndx-core/internal/pb/policy"

// Request/Response DTOs for V1 API endpoints

// CreateApplicationSubmissionRequest Consumer Application Submission DTOs
type CreateApplicationSubmissionRequest struct {
	ApplicationName        string                       `json:"applicationName" validate:"required"`
	ApplicationDescription *string                      `json:"applicationDescription,omitempty"`
	SelectedFields         []policy.SelectedFieldRecord `json:"selectedFields" validate:"required,min=1"`
	PreviousApplicationID  *string                      `json:"previousApplicationId,omitempty"`
	MemberID               string                       `json:"memberId" validate:"required"`
}

// UpdateApplicationSubmissionRequest updates the status of a consumer application submission
type UpdateApplicationSubmissionRequest struct {
	ApplicationName        *string                       `json:"applicationName,omitempty"`
	ApplicationDescription *string                       `json:"applicationDescription,omitempty"`
	SelectedFields         *[]policy.SelectedFieldRecord `json:"selectedFields,omitempty"`
	Status                 *string                       `json:"status,omitempty"`
	PreviousApplicationID  *string                       `json:"previousApplicationId,omitempty"`
	Review                 *string                       `json:"review,omitempty"`
}

// CreateApplicationRequest creates a new consumer application
type CreateApplicationRequest struct {
	ApplicationName        string                       `json:"applicationName" validate:"required"`
	ApplicationDescription *string                      `json:"applicationDescription,omitempty"`
	SelectedFields         []policy.SelectedFieldRecord `json:"selectedFields" validate:"required,min=1"`
	MemberID               string                       `json:"memberId" validate:"required"`
	// IdpApplicationID and IdpClientID let a caller register an application whose
	// OAuth2 client was already provisioned directly in the IDP (e.g. manually via
	// ThunderID's console). When both are set, creation skips calling the IDP
	// entirely and uses these values as-is. Must be provided together (both or
	// neither) - a single field alone is rejected.
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
	SelectedFields []policy.SelectedFieldRecord `json:"selectedFields" validate:"required,min=1"`
	GrantDuration  *policy.GrantDurationType    `json:"grantDuration,omitempty" validate:"omitempty,grant_duration_type_enum"`
}

type ApplicationResponse struct {
	ApplicationID          string                       `json:"applicationId"`
	ApplicationName        string                       `json:"applicationName"`
	ApplicationDescription *string                      `json:"applicationDescription,omitempty"`
	SelectedFields         []policy.SelectedFieldRecord `json:"selectedFields"`
	MemberID               string                       `json:"memberId"`
	Version                string                       `json:"version"`
	IdpApplicationID       *string                      `json:"idpApplicationId,omitempty"`
	IdpClientID            *string                      `json:"idpClientId,omitempty"`
	// IdpClientSecret is only set in the response that created the IDP client.
	// Portal Backend doesn't store it, and ThunderID doesn't return it again.
	IdpClientSecret *string `json:"idpClientSecret,omitempty"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

type ApplicationSubmissionResponse struct {
	SubmissionID           string                       `json:"submissionId"`
	PreviousApplicationID  *string                      `json:"previousApplicationId,omitempty"`
	ApplicationName        string                       `json:"applicationName"`
	ApplicationDescription *string                      `json:"applicationDescription,omitempty"`
	SelectedFields         []policy.SelectedFieldRecord `json:"selectedFields"`
	MemberID               string                       `json:"memberId"`
	Status                 string                       `json:"status"`
	CreatedAt              string                       `json:"createdAt"`
	UpdatedAt              string                       `json:"updatedAt"`
	Review                 *string                      `json:"review,omitempty"`
}
