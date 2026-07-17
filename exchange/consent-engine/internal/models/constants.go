package models

import "errors"

// ConsentStatus represents the status of a consent record
type ConsentStatus string

// ConsentStatus constants
const (
	StatusPending  ConsentStatus = "pending"
	StatusApproved ConsentStatus = "approved"
	StatusRejected ConsentStatus = "rejected"
	StatusExpired  ConsentStatus = "expired"
	StatusRevoked  ConsentStatus = "revoked"
)

// ConsentType represents the type of consent mechanism
type ConsentType string

// ConsentType constants
const (
	TypeRealtime ConsentType = "realtime"
	TypeOffline  ConsentType = "offline"
)

// ConsentPortalAction represents the action taken in the consent portal
type ConsentPortalAction string

// ConsentPortalAction constants
const (
	ActionApprove ConsentPortalAction = "approve"
	ActionReject  ConsentPortalAction = "reject"
)

// GrantDuration represents the duration for which consent is granted
type GrantDuration string

// GrantDuration constants
const (
	DurationOneHour     GrantDuration = "PT1H"          // 1 hour
	DurationSixHours    GrantDuration = "PT6H"          // 6 hours
	DurationTwelveHours GrantDuration = "PT12H"         // 12 hours
	DurationOneDay      GrantDuration = "P1D"           // 1 day
	DurationSevenDays   GrantDuration = "P7D"           // 7 days
	DurationThirtyDays  GrantDuration = "P30D"          // 30 days
	DurationDefault     GrantDuration = DurationOneHour // default duration
)

// DefaultPendingTimeoutDuration represents the default duration for pending status expiry
// based on consent type. Pending consents will expire after this duration if not approved or rejected
// Format: ISO 8601 duration (e.g., "P1D" for 1 day, "PT24H" for 24 hours)
const (
	DefaultPendingTimeoutRealtimeDuration = "PT1H" // 1 hour for realtime consent
	DefaultPendingTimeoutOfflineDuration  = "P1D"  // 1 day for offline consent
)

// OwnerType represents the owner enum (matches PolicyDecisionPoint Owner type)
type OwnerType string

const (
	OwnerCitizen OwnerType = "citizen"
)

// Sentinel errors for consent operations
// These errors can be checked using errors.Is()
var (
	ErrConsentNotFound     = errors.New("consent record not found")
	ErrConsentCreateFailed = errors.New("failed to create consent record")
	ErrConsentUpdateFailed = errors.New("failed to update consent record")
	ErrConsentRevokeFailed = errors.New("failed to revoke consent record")
	ErrConsentGetFailed    = errors.New("failed to get consent records")
	ErrConsentExpiryFailed = errors.New("failed to check consent expiry")
	ErrPortalRequestFailed = errors.New("failed to process consent portal request")
)

// ConsentErrorCode represents an error code
type ConsentErrorCode string

// ConsentErrorCode constants
const (
	ErrorCodeConsentNotFound  ConsentErrorCode = "CONSENT_NOT_FOUND"
	ErrorCodeInternalError    ConsentErrorCode = "INTERNAL_ERROR"
	ErrorCodeBadRequest       ConsentErrorCode = "BAD_REQUEST"
	ErrorCodeUnauthorized     ConsentErrorCode = "UNAUTHORIZED"
	ErrorCodeForbidden        ConsentErrorCode = "FORBIDDEN"
	ErrorCodeMethodNotAllowed ConsentErrorCode = "METHOD_NOT_ALLOWED"
)

// ConsentEngineOperation represents the operation
type ConsentEngineOperation string

// ConsentEngineOperation constants
const (
	OpCreateConsent         ConsentEngineOperation = "create consent"
	OpUpdateConsent         ConsentEngineOperation = "update consent"
	OpRevokeConsent         ConsentEngineOperation = "revoke consent"
	OpGetConsentStatus      ConsentEngineOperation = "get consent status"
	OpGetConsentsByOwner    ConsentEngineOperation = "get consents by data owner"
	OpGetConsentsByConsumer ConsentEngineOperation = "get consents by consumer"
	OpCheckConsentExpiry    ConsentEngineOperation = "check consent expiry"
	OpProcessPortalRequest  ConsentEngineOperation = "process consent portal"
)

// UpdateByMessage represents who updated the consent with specific message
type UpdateByMessage string

// UpdateByMessage constants
const (
	RevokedByNewConsentWithDifferentFields UpdateByMessage = "System: revoked due to new consent with different fields"
)
