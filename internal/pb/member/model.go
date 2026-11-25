// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package member is the Portal Backend's member bounded context: member
// organizations, their link to the IdP, and the member API.
package member

import "github.com/openndx/openndx-core/internal/pb/kernel"

// Member represents a member organization. Its users live in the IdP, under
// the ThunderID organization unit identified by OuID. Email and PhoneNumber
// are optional, shared contact channels for the organization (e.g. a team
// mailbox), not a person's details.
type Member struct {
	MemberID    string `gorm:"primarykey;column:member_id" json:"memberId"`
	Name        string `gorm:"column:name;not null" json:"name"`
	Email       string `gorm:"column:email" json:"email,omitempty"`
	PhoneNumber string `gorm:"column:phone_number" json:"phoneNumber,omitempty"`
	OuID        string `gorm:"column:ou_id;not null;unique" json:"ouId"`
	kernel.BaseModel
}

// TableName sets the table name for GORM
func (Member) TableName() string {
	return "members"
}
