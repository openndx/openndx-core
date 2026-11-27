// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package application is the Portal Backend's consumer application bounded
// context: applications, application submissions, their IdP clients and PDP
// allow-lists, and the application API.
package application

import (
	"github.com/openndx/openndx-core/internal/pb/kernel"
	"github.com/openndx/openndx-core/internal/pb/member"
)

// IDP Application Constants
const (
	TemplateIDM2M = "m2m-application"
)

// Application represents the consumer_applications table
type Application struct {
	ApplicationID          string               `gorm:"primarykey;column:application_id" json:"applicationId"`
	ApplicationName        string               `gorm:"column:application_name;not null" json:"applicationName"`
	ApplicationDescription *string              `gorm:"column:application_description" json:"applicationDescription,omitempty"`
	SelectedFields         SelectedFieldRecords `gorm:"column:selected_fields;type:jsonb;not null" json:"selectedFields"`
	MemberID               string               `gorm:"column:member_id;not null" json:"memberId"`
	Version                string               `gorm:"column:version;not null" json:"version"`
	IdpApplicationID       *string              `gorm:"column:idp_application_id" json:"idpApplicationId,omitempty"`
	IdpClientID            *string              `gorm:"column:idp_client_id" json:"idpClientId,omitempty"`
	kernel.BaseModel

	// Relationships
	Member member.Member `gorm:"foreignKey:MemberID;references:MemberID" json:"member"`
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
	kernel.BaseModel

	// Relationships
	Member              member.Member `gorm:"foreignKey:MemberID;references:MemberID" json:"member"`
	PreviousApplication *Application  `gorm:"foreignKey:PreviousApplicationID;references:ApplicationID" json:"previousApplication,omitempty"`
}

// TableName sets the table name for GORM
func (ApplicationSubmission) TableName() string {
	return "application_submissions"
}
