package models

import (
	"time"

	"github.com/google/uuid"
)

// ConsentRecord represents a consent record in the system
// Business Rules:
// - Only one record can exist with status 'pending' or 'approved' for a given (OwnerID, OwnerEmail, AppID) tuple
// - Multiple records can exist with status 'revoked', 'expired', or 'rejected' for the same tuple
// - The most recently created record should have status 'pending' or 'approved' (if active)
type ConsentRecord struct {
	// ConsentID is the unique identifier for the consent record
	ConsentID uuid.UUID `gorm:"column:consent_id;type:uuid;primaryKey;default:gen_random_uuid()" json:"consent_id"`
	// OwnerID is the unique identifier for the data owner
	// Part of conditional unique constraint for active consents (pending/approved)
	OwnerID string `gorm:"column:owner_id;type:varchar(255);not null;index:idx_consent_records_owner_id;index:idx_consent_records_owner_app;uniqueIndex:idx_consent_active_unique,where:status = 'pending' OR status = 'approved'" json:"owner_id"`
	// OwnerEmail is the email address of the data owner
	// Part of conditional unique constraint for active consents (pending/approved)
	OwnerEmail string `gorm:"column:owner_email;type:varchar(255);not null;index:idx_consent_records_owner_email;uniqueIndex:idx_consent_active_unique,where:status = 'pending' OR status = 'approved'" json:"owner_email"`
	// AppID is the unique identifier for the consumer application
	// Part of conditional unique constraint for active consents (pending/approved)
	AppID string `gorm:"column:app_id;type:varchar(255);not null;index:idx_consent_records_app_id;index:idx_consent_records_owner_app;uniqueIndex:idx_consent_active_unique,where:status = 'pending' OR status = 'approved'" json:"app_id"`
	// AppName is the name of the consumer application
	AppName *string `gorm:"column:app_name;type:varchar(255);" json:"app_name,omitempty"`
	// Status is the status of the consent record: pending, approved, rejected, expired, revoked
	// Part of conditional unique constraint for active consents (pending/approved)
	Status string `gorm:"column:status;type:varchar(50);not null;index:idx_consent_records_status;uniqueIndex:idx_consent_active_unique,where:status = 'pending' OR status = 'approved'" json:"status"`
	// Type is the type of consent mechanism "realtime" or "offline"
	Type string `gorm:"column:type;type:varchar(50);not null" json:"type"`
	// CreatedAt is the timestamp when the consent record was created
	// Used to determine the most recent active consent
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp with time zone;not null;default:CURRENT_TIMESTAMP;index:idx_consent_records_created_at" json:"created_at"`
	// UpdatedAt is the timestamp when the consent record was last updated
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamp with time zone;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	// PendingExpiresAt is the timestamp when a pending consent expires (timeout waiting for approval/denial)
	// Set when status is pending, cleared when status changes to approved/rejected
	PendingExpiresAt *time.Time `gorm:"column:pending_expires_at;type:timestamp with time zone;index:idx_consent_records_pending_expires_at" json:"pending_expires_at,omitempty"`
	// GrantExpiresAt is the timestamp when an approved consent grant expires
	// Calculated by adding GrantDuration to the current time when consent is approved
	// Only set when status is approved
	GrantExpiresAt *time.Time `gorm:"column:grant_expires_at;type:timestamp with time zone;index:idx_consent_records_grant_expires_at" json:"grant_expires_at,omitempty"`
	// GrantDuration is the duration to add to current time when approving consent (e.g., "P30D", "1h")
	// Used to calculate GrantExpiresAt: GrantExpiresAt = current_time + GrantDuration
	GrantDuration string `gorm:"column:grant_duration;type:varchar(50);not null" json:"grant_duration"`
	// Fields is the list of data fields that require consent (stored as array of field names)
	Fields []ConsentField `gorm:"column:fields;type:jsonb;serializer:json;not null" json:"fields"`
	// SessionID is the session identifier for tracking the consent flow
	SessionID *string `gorm:"column:session_id;type:varchar(255);" json:"session_id,omitempty"`
	// ConsentPortalURL is the URL to redirect to for consent portal
	ConsentPortalURL string `gorm:"column:consent_portal_url;type:text;not null" json:"consent_portal_url"`
	// UpdatedBy identifies who last updated the consent (audit field)
	UpdatedBy *string `gorm:"column:updated_by;type:varchar(255)" json:"updated_by,omitempty"`
}

// TableName specifies the table name for GORM
func (*ConsentRecord) TableName() string {
	return "consent_records"
}
