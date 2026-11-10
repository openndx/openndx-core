package models

import (
	"time"

	"github.com/google/uuid"
)

// GrantStatus is the lifecycle state of an access grant.
type GrantStatus string

const (
	// GrantStatusActive grants the application access to the field until the
	// grant expires.
	GrantStatusActive GrantStatus = "active"
	// GrantStatusRevoked no longer grants access. The row is kept as a record.
	GrantStatusRevoked GrantStatus = "revoked"
)

// IsValid reports whether s is a known grant status.
func (s GrantStatus) IsValid() bool {
	switch s {
	case GrantStatusActive, GrantStatusRevoked:
		return true
	}
	return false
}

// AccessGrant represents the access_grants table: one application's access to
// one field. A field has at most one active grant per application.
type AccessGrant struct {
	ID            uuid.UUID   `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	SchemaID      string      `gorm:"column:schema_id;not null"`
	FieldName     string      `gorm:"column:field_name;not null"`
	ApplicationID string      `gorm:"column:application_id;not null"`
	ExpiresAt     time.Time   `gorm:"column:expires_at;not null"`
	Status        GrantStatus `gorm:"column:status;not null"`
	CreatedAt     time.Time   `gorm:"column:created_at;not null"`
	UpdatedAt     time.Time   `gorm:"column:updated_at;not null"`
}

// TableName specifies the table name for GORM
func (AccessGrant) TableName() string {
	return "access_grants"
}

// ToResponse converts AccessGrant to AccessGrantResponse
func (g *AccessGrant) ToResponse() AccessGrantResponse {
	return AccessGrantResponse{
		ID:            g.ID.String(),
		SchemaID:      g.SchemaID,
		FieldName:     g.FieldName,
		ApplicationID: g.ApplicationID,
		ExpiresAt:     g.ExpiresAt.UTC().Format(time.RFC3339),
		Status:        g.Status,
		CreatedAt:     g.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     g.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
