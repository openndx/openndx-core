// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package member

type CreateMemberRequest struct {
	Name string `json:"name" validate:"required"`
	// Email and PhoneNumber are optional contact channels for the organization
	Email       string `json:"email,omitempty" validate:"omitempty,email"`
	PhoneNumber string `json:"phoneNumber,omitempty"`
	// OuID is the ThunderID organization unit this member maps to. The OU and
	// its users are managed in ThunderID; PB only stores the link.
	OuID string `json:"ouId" validate:"required"`
}

type UpdateMemberRequest struct {
	Name        *string `json:"name,omitempty"`
	Email       *string `json:"email,omitempty" validate:"omitempty,email"`
	PhoneNumber *string `json:"phoneNumber,omitempty"`
}

type MemberResponse struct {
	MemberID    string `json:"memberId"`
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	PhoneNumber string `json:"phoneNumber,omitempty"`
	OuID        string `json:"ouId"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}
