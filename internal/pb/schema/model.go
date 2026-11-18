// Package schema is the Portal Backend's provider schema bounded context:
// schemas, schema submissions, their PDP field policies, and the schema API.
package schema

import (
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/member"
)

// Schema represents the provider_schemas table
type Schema struct {
	SchemaID          string  `gorm:"primarykey;column:schema_id" json:"schemaId"`
	MemberID          string  `gorm:"column:member_id;not null" json:"memberId"`
	SchemaName        string  `gorm:"column:schema_name;not null" json:"schemaName"`
	SDL               string  `gorm:"column:sdl;not null" json:"sdl"`
	Endpoint          string  `gorm:"column:endpoint;not null" json:"endpoint"`
	Version           string  `gorm:"column:version;not null" json:"version"`
	SchemaDescription *string `gorm:"column:schema_description" json:"schemaDescription,omitempty"`
	kernel.BaseModel

	// Relationships
	Member member.Member `gorm:"foreignKey:MemberID;references:MemberID" json:"member"`
}

// TableName sets the table name for GORM
func (Schema) TableName() string {
	return "schemas"
}

// SchemaSubmission represents the provider_schema_submissions table
type SchemaSubmission struct {
	SubmissionID      string  `gorm:"primarykey;column:submission_id" json:"submissionId"`
	PreviousSchemaID  *string `gorm:"column:previous_schema_id" json:"previousSchemaId,omitempty"`
	SchemaName        string  `gorm:"column:schema_name;not null" json:"schemaName"`
	SchemaDescription *string `gorm:"column:schema_description" json:"schemaDescription,omitempty"`
	SDL               string  `gorm:"column:sdl;not null" json:"sdl"`
	SchemaEndpoint    string  `gorm:"column:schema_endpoint;not null" json:"schemaEndpoint"`
	Status            string  `gorm:"column:status;not null" json:"status"`
	MemberID          string  `gorm:"column:member_id;not null" json:"memberId"`
	Review            *string `gorm:"column:review" json:"review,omitempty"`
	kernel.BaseModel

	// Relationships
	Member         member.Member `gorm:"foreignKey:MemberID;references:MemberID" json:"member"`
	PreviousSchema *Schema       `gorm:"foreignKey:PreviousSchemaID;references:SchemaID" json:"previousSchema,omitempty"`
}

// TableName sets the table name for GORM
func (SchemaSubmission) TableName() string {
	return "schema_submissions"
}
