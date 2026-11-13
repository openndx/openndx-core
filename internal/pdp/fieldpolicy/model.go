package fieldpolicy

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openndx/openndx-core/internal/pdp/decision"
)

// Policy is one field's access policy, stored in the policy_metadata table.
type Policy struct {
	ID          uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SchemaID    string    `gorm:"column:schema_id;type:varchar(255);not null;uniqueIndex:idx_policy_metadata_schema_field;" json:"schemaId"`
	FieldName   string    `gorm:"column:field_name;type:text;not null;uniqueIndex:idx_policy_metadata_schema_field" json:"fieldName"`
	DisplayName *string   `gorm:"column:display_name;type:text" json:"displayName,omitempty"`
	Description *string   `gorm:"column:description;type:text" json:"description,omitempty"`
	Source      Source    `gorm:"column:source;type:source_enum;not null;default:'fallback'" json:"source"`
	// The flags have no column default: a GORM default would skip writing false.
	RequiresAllowlist bool      `gorm:"column:requires_allowlist;not null" json:"requiresAllowlist"`
	RequiresConsent   bool      `gorm:"column:requires_consent;not null" json:"requiresConsent"`
	Owner             *Owner    `gorm:"column:owner;type:owner_enum;" json:"owner"`
	CreatedAt         time.Time `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP;not null" json:"createdAt"`
	UpdatedAt         time.Time `gorm:"column:updated_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName specifies the table name for GORM
func (Policy) TableName() string {
	return "policy_metadata"
}

// ToResponse converts Policy to PolicyResponse
func (p *Policy) ToResponse() PolicyResponse {
	return PolicyResponse{
		ID:                p.ID.String(),
		SchemaID:          p.SchemaID,
		FieldName:         p.FieldName,
		DisplayName:       p.DisplayName,
		Description:       p.Description,
		Source:            p.Source,
		RequiresAllowlist: p.RequiresAllowlist,
		RequiresConsent:   p.RequiresConsent,
		Owner:             p.Owner,
		CreatedAt:         p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         p.UpdatedAt.Format(time.RFC3339),
	}
}

// toFieldPolicy converts Policy to the policy a decision reads.
func (p *Policy) toFieldPolicy() decision.FieldPolicy {
	return decision.FieldPolicy{
		FieldKey: decision.FieldKey{SchemaID: p.SchemaID, FieldName: p.FieldName},
		Policy: decision.Field{
			RequiresAllowlist: p.RequiresAllowlist,
			RequiresConsent:   p.RequiresConsent,
		},
		DisplayName: p.DisplayName,
		Description: p.Description,
		Owner:       (*string)(p.Owner),
	}
}

// Source represents the source enum
type Source string

const (
	SourcePrimary  Source = "primary"
	SourceFallback Source = "fallback"
)

// Scan implements the sql.Scanner interface for Source
func (s *Source) Scan(value any) error {
	if value == nil {
		*s = SourceFallback
		return nil
	}
	if str, ok := value.(string); ok {
		*s = Source(str)
		return nil
	}
	return fmt.Errorf("cannot scan %T into Source", value)
}

// Value implements the driver.Valuer interface for Source
func (s Source) Value() (driver.Value, error) {
	return string(s), nil
}

// Owner represents the owner enum
type Owner string

const (
	OwnerCitizen Owner = "citizen"
)

// Scan implements the sql.Scanner interface for Owner
func (o *Owner) Scan(value any) error {
	if value == nil {
		*o = OwnerCitizen
		return nil
	}
	if str, ok := value.(string); ok {
		*o = Owner(str)
		return nil
	}
	return fmt.Errorf("cannot scan %T into Owner", value)
}

// Value implements the driver.Valuer interface for Owner
func (o Owner) Value() (driver.Value, error) {
	return string(o), nil
}
