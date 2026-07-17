package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PolicyMetadata represents the policy_metadata table
type PolicyMetadata struct {
	ID                uuid.UUID         `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SchemaID          string            `gorm:"column:schema_id;type:varchar(255);not null;uniqueIndex:idx_policy_metadata_schema_field;" json:"schemaId"`
	FieldName         string            `gorm:"column:field_name;type:text;not null;uniqueIndex:idx_policy_metadata_schema_field" json:"fieldName"`
	DisplayName       *string           `gorm:"column:display_name;type:text" json:"displayName,omitempty"`
	Description       *string           `gorm:"column:description;type:text" json:"description,omitempty"`
	Source            Source            `gorm:"column:source;type:source_enum;not null;default:'fallback'" json:"source"`
	IsOwner           bool              `gorm:"column:is_owner;type:boolean;default:false;not null" json:"isOwner"`
	AccessControlType AccessControlType `gorm:"column:access_control_type;type:access_control_type_enum;not null;default:'restricted'" json:"accessControlType"`
	AllowList         AllowList         `gorm:"column:allow_list;type:jsonb;not null;default:'{}'" json:"allowList"`
	Owner             *Owner            `gorm:"column:owner;type:owner_enum;" json:"owner"`
	CreatedAt         time.Time         `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP;not null" json:"createdAt"`
	UpdatedAt         time.Time         `gorm:"column:updated_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName specifies the table name for GORM
func (PolicyMetadata) TableName() string {
	return "policy_metadata"
}

// BeforeCreate validates the model before creating
func (pm *PolicyMetadata) BeforeCreate(tx *gorm.DB) error {
	return pm.validateOwnerConstraint()
}

// BeforeUpdate validates the model before updating
func (pm *PolicyMetadata) BeforeUpdate(tx *gorm.DB) error {
	return pm.validateOwnerConstraint()
}

// validateOwnerConstraint ensures that if isOwner is false, owner cannot be null
func (pm *PolicyMetadata) validateOwnerConstraint() error {
	if (!pm.IsOwner && pm.Owner == nil) || (pm.IsOwner && pm.Owner != nil) {
		return errors.New("owner must be specified when isOwner is false and must be null when isOwner is true")
	}
	return nil
}

// ToResponse converts PolicyMetadata to PolicyMetadataResponse
func (pm *PolicyMetadata) ToResponse() PolicyMetadataResponse {
	return PolicyMetadataResponse{
		ID:                pm.ID.String(),
		SchemaID:          pm.SchemaID,
		FieldName:         pm.FieldName,
		DisplayName:       pm.DisplayName,
		Description:       pm.Description,
		Source:            pm.Source,
		IsOwner:           pm.IsOwner,
		AccessControlType: pm.AccessControlType,
		AllowList:         pm.AllowList,
		Owner:             pm.Owner,
		CreatedAt:         pm.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         pm.UpdatedAt.Format(time.RFC3339),
	}
}
