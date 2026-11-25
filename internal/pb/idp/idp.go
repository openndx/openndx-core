// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package idp

import "context"

type IdentityProviderAPI interface {
	UserManager
	ApplicationManager
	GroupManager
}

type UserManager interface {
	CreateUser(ctx context.Context, userInfo *User) (*UserInfo, error)
	GetUser(ctx context.Context, userId string) (*UserInfo, error)
	UpdateUser(ctx context.Context, userId string, userInfo *User) (*UserInfo, error)
	DeleteUser(ctx context.Context, userId string) error
}

// ApplicationManager is what Portal Backend needs from the IdP: creating a
// consumer application's OAuth2 client, and deleting it again on rollback.
// internal/pb/idp/thunderid implements it.
type ApplicationManager interface {
	CreateApplication(ctx context.Context, app *Application) (*CreatedApplication, error)
	DeleteApplication(ctx context.Context, applicationId string) error
}

type User struct {
	FirstName   string
	LastName    string
	Email       string
	PhoneNumber string
}

type UserInfo struct {
	Id          string
	Email       string
	FirstName   string
	LastName    string
	PhoneNumber string
}

type Application struct {
	Name        string
	Description string
	TemplateId  string
	// OuID is the ThunderID organization unit the application is created in:
	// the owning member's organization unit.
	OuID string
}

// CreatedApplication is an application just created in the IdP. ClientSecret
// may only be readable here: ThunderID doesn't return it again afterwards.
type CreatedApplication struct {
	Id           string
	ClientId     string
	ClientSecret string
}

type ApplicationInfo struct {
	Id          string
	Name        string
	Description string
	ClientId    string
}

type ApplicationOIDCInfo struct {
	ClientId     string
	ClientSecret string
}

type GroupManager interface {
	CreateGroup(ctx context.Context, group *Group) (*GroupInfo, error)
	GetGroup(ctx context.Context, groupId string) (*GroupInfo, error)
	GetGroupByName(ctx context.Context, groupName string) (*string, error)
	UpdateGroup(ctx context.Context, groupId string, group *Group) (*GroupInfo, error)
	DeleteGroup(ctx context.Context, groupId string) error
	AddMemberToGroup(ctx context.Context, groupId string, memberInfo *GroupMember) error
	AddMemberToGroupByGroupName(ctx context.Context, groupName string, memberInfo *GroupMember) (*string, error) // Returns groupId
	RemoveMemberFromGroup(ctx context.Context, groupId string, userId string) error
}

type Group struct {
	DisplayName string
	Members     []*GroupMember
}

type GroupMember struct {
	Value   string
	Display string
}

type GroupInfo struct {
	Id          string
	DisplayName string
	Members     []GroupMember
}
