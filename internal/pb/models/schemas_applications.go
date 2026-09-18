package models

// Schema represents the provider_schemas table
type Schema struct {
	SchemaID          string  `gorm:"primarykey;column:schema_id" json:"schemaId"`
	MemberID          string  `gorm:"column:member_id;not null" json:"memberId"`
	SchemaName        string  `gorm:"column:schema_name;not null" json:"schemaName"`
	SDL               string  `gorm:"column:sdl;not null" json:"sdl"`
	Endpoint          string  `gorm:"column:endpoint;not null" json:"endpoint"`
	Version           string  `gorm:"column:version;not null" json:"version"`
	SchemaDescription *string `gorm:"column:schema_description" json:"schemaDescription,omitempty"`
	BaseModel

	// Relationships
	Member Member `gorm:"foreignKey:MemberID;references:MemberID" json:"member"`
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
	BaseModel

	// Relationships
	Member         Member  `gorm:"foreignKey:MemberID;references:MemberID" json:"member"`
	PreviousSchema *Schema `gorm:"foreignKey:PreviousSchemaID;references:SchemaID" json:"previousSchema,omitempty"`
}

// TableName sets the table name for GORM
func (SchemaSubmission) TableName() string {
	return "schema_submissions"
}

// Application represents the consumer_applications table
type Application struct {
	ApplicationID          string               `gorm:"primarykey;column:application_id" json:"applicationId"`
	ApplicationName        string               `gorm:"column:application_name;not null" json:"applicationName"`
	ApplicationDescription *string              `gorm:"column:application_description" json:"applicationDescription,omitempty"`
	SelectedFields         SelectedFieldRecords `gorm:"column:selected_fields;type:jsonb;not null" json:"selectedFields"`
	MemberID               string               `gorm:"column:member_id;not null" json:"memberId"`
	Version                string               `gorm:"column:version;not null" json:"version"`
	IdpApplicationID       *string              `gorm:"column:idp_application_id" json:"idpApplicationId,omitempty"` // Until the data migration is done this can be nullable
	IdpClientID            *string              `gorm:"column:idp_client_id" json:"idpClientId,omitempty"`           // Until the data migration is done this can be nullable
	BaseModel

	// Relationships
	Member Member `gorm:"foreignKey:MemberID;references:MemberID" json:"member"`
}

// TableName sets the table name for GORM
func (Application) TableName() string {
	return "applications"
}

// ApplicationSubmission represents the consumer_application_submissions table
type ApplicationSubmission struct {
	SubmissionID           string               `gorm:"primarykey;column:submission_id" json:"submissionId"`
	PreviousApplicationID  *string              `gorm:"column:previous_application_id" json:"previousApplicationId,omitempty"`
	ApplicationName        string               `gorm:"column:application_name;not null" json:"applicationName"`
	ApplicationDescription *string              `gorm:"column:application_description" json:"applicationDescription,omitempty"`
	SelectedFields         SelectedFieldRecords `gorm:"column:selected_fields;type:jsonb;not null" json:"selectedFields"`
	MemberID               string               `gorm:"column:member_id;not null" json:"memberId"`
	Status                 string               `gorm:"column:status;not null" json:"status"`
	Review                 *string              `gorm:"column:review" json:"review,omitempty"`
	BaseModel

	// Relationships
	Member              Member       `gorm:"foreignKey:MemberID;references:MemberID" json:"member"`
	PreviousApplication *Application `gorm:"foreignKey:PreviousApplicationID;references:ApplicationID" json:"previousApplication,omitempty"`
}

// TableName sets the table name for GORM
func (ApplicationSubmission) TableName() string {
	return "application_submissions"
}
