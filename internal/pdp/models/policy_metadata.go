package models

import (
	"time"

	"github.com/google/uuid"
)

// PolicyMetadata represents the policy_metadata table
type PolicyMetadata struct {
	ID           uuid.UUID    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SchemaID     string       `gorm:"column:schema_id;type:varchar(255);not null;uniqueIndex:idx_policy_metadata_schema_field;" json:"schemaId"`
	FieldName    string       `gorm:"column:field_name;type:text;not null;uniqueIndex:idx_policy_metadata_schema_field" json:"fieldName"`
	DisplayName  *string      `gorm:"column:display_name;type:text" json:"displayName,omitempty"`
	Description  *string      `gorm:"column:description;type:text" json:"description,omitempty"`
	Source       Source       `gorm:"column:source;type:source_enum;not null;default:'fallback'" json:"source"`
	PolicyClass  string       `gorm:"column:policy_class;type:text;not null" json:"policyClass"`
	PolicyParams PolicyParams `gorm:"column:policy_params;type:jsonb;not null;default:'{}'" json:"policyParams"`
	AllowList    AllowList    `gorm:"column:allow_list;type:jsonb;not null;default:'{}'" json:"allowList"`
	Owner        *Owner       `gorm:"column:owner;type:owner_enum;" json:"owner"`
	CreatedAt    time.Time    `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP;not null" json:"createdAt"`
	UpdatedAt    time.Time    `gorm:"column:updated_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName specifies the table name for GORM
func (PolicyMetadata) TableName() string {
	return "policy_metadata"
}

// ToResponse converts PolicyMetadata to PolicyMetadataResponse
func (pm *PolicyMetadata) ToResponse() PolicyMetadataResponse {
	policyParams := pm.PolicyParams
	if policyParams == nil {
		policyParams = PolicyParams{}
	}
	return PolicyMetadataResponse{
		ID:           pm.ID.String(),
		SchemaID:     pm.SchemaID,
		FieldName:    pm.FieldName,
		DisplayName:  pm.DisplayName,
		Description:  pm.Description,
		Source:       pm.Source,
		PolicyClass:  pm.PolicyClass,
		PolicyParams: policyParams,
		AllowList:    pm.AllowList,
		Owner:        pm.Owner,
		CreatedAt:    pm.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    pm.UpdatedAt.Format(time.RFC3339),
	}
}
