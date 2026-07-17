package models

import (
	"time"
)

// ConsentField represents a field that requires consent
// Matches PolicyDecisionResponseFieldRecord DTO structure from PolicyDecisionPoint
type ConsentField struct {
	FieldName   string    `json:"fieldName"`
	SchemaID    string    `json:"schemaId"`
	DisplayName *string   `json:"displayName,omitempty"`
	Description *string   `json:"description,omitempty"`
	Owner       OwnerType `json:"owner"`
}

// Equals checks if two ConsentField instances are equal
func (a ConsentField) Equals(b ConsentField) bool {
	return a.FieldName == b.FieldName &&
		a.SchemaID == b.SchemaID &&
		((a.DisplayName == nil && b.DisplayName == nil) || (a.DisplayName != nil && b.DisplayName != nil && *a.DisplayName == *b.DisplayName)) &&
		((a.Description == nil && b.Description == nil) || (a.Description != nil && b.Description != nil && *a.Description == *b.Description)) &&
		a.Owner == b.Owner
}

// ConsentRequirement represents a consent requirement for a specific owner
type ConsentRequirement struct {
	Owner   OwnerType      `json:"owner"`
	OwnerID string         `json:"ownerId"`
	Fields  []ConsentField `json:"fields"`
}

// CreateConsentRequest defines the structure for creating a consent record
// GrantDuration is optional - nil means not provided and will use default value
type CreateConsentRequest struct {
	AppID              string             `json:"appId"`
	AppName            *string            `json:"appName,omitempty"`
	ConsentRequirement ConsentRequirement `json:"consentRequirement"`
	GrantDuration      *string            `json:"grantDuration,omitempty"`
	ConsentType        *ConsentType       `json:"consentType,omitempty"`
}

// ConsentPortalActionRequest defines the structure for consent portal interactions
type ConsentPortalActionRequest struct {
	ConsentID string              `json:"consentId"`
	Action    ConsentPortalAction `json:"action"` // "approve" or "reject"
	UpdatedBy string              `json:"updatedBy"`
}

// ConsentResponseInternalView represents a simplified consent response structure for Internal API Responses
type ConsentResponseInternalView struct {
	ConsentID        string          `json:"consentId"`
	Status           string          `json:"status"`
	ConsentPortalURL *string         `json:"consentPortalUrl,omitempty"` // Only present when status is pending
	Fields           *[]ConsentField `json:"fields,omitempty"`           // Included for internal use if needed
}

// ConsentResponsePortalView represents the user-facing consent object for the UI.
// Uses rich field information for better UX in the consent portal
type ConsentResponsePortalView struct {
	AppID     string         `json:"appId"`
	AppName   *string        `json:"appName"`
	OwnerID   string         `json:"ownerId"`
	Status    ConsentStatus  `json:"status"`
	Type      ConsentType    `json:"type"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Fields    []ConsentField `json:"fields"` // Rich field information with display names and descriptions
}

// ToConsentResponseInternalView converts a ConsentRecord to a simplified ConsentResponseInternalView.
// Only includes consent_portal_url when status is pending and the URL is not empty
// Includes fields only when status is pending or approved to support internal operations
func (cr *ConsentRecord) ToConsentResponseInternalView() ConsentResponseInternalView {
	response := ConsentResponseInternalView{
		ConsentID: cr.ConsentID.String(),
		Status:    cr.Status,
	}

	// Only include consent_portal_url when status is pending and URL is not empty
	if cr.Status == string(StatusPending) && cr.ConsentPortalURL != "" {
		portalURL := cr.ConsentPortalURL
		response.ConsentPortalURL = &portalURL
	}

	if cr.Status == string(StatusPending) || cr.Status == string(StatusApproved) {
		response.Fields = &cr.Fields
	}

	return response
}

// ToConsentResponsePortalView converts an internal ConsentRecord to a user-facing view.
// Returns rich field information including display names and descriptions for better UX
func (cr *ConsentRecord) ToConsentResponsePortalView() ConsentResponsePortalView {
	return ConsentResponsePortalView{
		AppID:     cr.AppID,
		AppName:   cr.AppName,
		OwnerID:   cr.OwnerID,
		Status:    ConsentStatus(cr.Status),
		Type:      ConsentType(cr.Type),
		CreatedAt: cr.CreatedAt,
		UpdatedAt: cr.UpdatedAt,
		Fields:    cr.Fields, // Now includes DisplayName, Description, and Owner for rich UI rendering
	}
}
