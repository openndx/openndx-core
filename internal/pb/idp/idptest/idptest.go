// Package idptest provides a fake idp.IdentityProviderAPI for Portal Backend
// tests. Import it only from _test.go files.
package idptest

import (
	"context"

	"github.com/openndx/openndx-core/internal/pb/idp"
)

// Mock is a fake identity provider for testing. Each method calls the matching
// Func field when set, and otherwise returns a successful default.
type Mock struct {
	CreateUserFunc                  func(ctx context.Context, user *idp.User) (*idp.UserInfo, error)
	UpdateUserFunc                  func(ctx context.Context, userID string, user *idp.User) (*idp.UserInfo, error)
	DeleteUserFunc                  func(ctx context.Context, userID string) error
	AddMemberToGroupByGroupNameFunc func(ctx context.Context, groupName string, member *idp.GroupMember) (*string, error)
	RemoveMemberFromGroupFunc       func(ctx context.Context, groupID string, userID string) error
	GetUserFunc                     func(ctx context.Context, userID string) (*idp.UserInfo, error)
	GetGroupFunc                    func(ctx context.Context, groupID string) (*idp.GroupInfo, error)
	GetGroupByNameFunc              func(ctx context.Context, groupName string) (*string, error)
	CreateGroupFunc                 func(ctx context.Context, group *idp.Group) (*idp.GroupInfo, error)
	UpdateGroupFunc                 func(ctx context.Context, groupID string, group *idp.Group) (*idp.GroupInfo, error)
	AddMemberToGroupFunc            func(ctx context.Context, groupID string, memberInfo *idp.GroupMember) error
	CreateApplicationFunc           func(ctx context.Context, app *idp.Application) (*string, error)
	DeleteApplicationFunc           func(ctx context.Context, applicationID string) error
	DeleteGroupFunc                 func(ctx context.Context, groupID string) error
	GetApplicationInfoFunc          func(ctx context.Context, applicationID string) (*idp.ApplicationInfo, error)
	GetApplicationOIDCFunc          func(ctx context.Context, applicationID string) (*idp.ApplicationOIDCInfo, error)
}

var _ idp.IdentityProviderAPI = (*Mock)(nil)

func (m *Mock) CreateUser(ctx context.Context, user *idp.User) (*idp.UserInfo, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, user)
	}
	return &idp.UserInfo{Id: "idp_123", Email: user.Email}, nil
}

func (m *Mock) UpdateUser(ctx context.Context, userID string, user *idp.User) (*idp.UserInfo, error) {
	if m.UpdateUserFunc != nil {
		return m.UpdateUserFunc(ctx, userID, user)
	}
	return &idp.UserInfo{Id: userID, Email: user.Email}, nil
}

func (m *Mock) DeleteUser(ctx context.Context, userID string) error {
	if m.DeleteUserFunc != nil {
		return m.DeleteUserFunc(ctx, userID)
	}
	return nil
}

func (m *Mock) AddMemberToGroupByGroupName(ctx context.Context, groupName string, member *idp.GroupMember) (*string, error) {
	if m.AddMemberToGroupByGroupNameFunc != nil {
		return m.AddMemberToGroupByGroupNameFunc(ctx, groupName, member)
	}
	groupID := "group_123"
	return &groupID, nil
}

func (m *Mock) RemoveMemberFromGroup(ctx context.Context, groupID string, userID string) error {
	if m.RemoveMemberFromGroupFunc != nil {
		return m.RemoveMemberFromGroupFunc(ctx, groupID, userID)
	}
	return nil
}

func (m *Mock) GetUser(ctx context.Context, userID string) (*idp.UserInfo, error) {
	if m.GetUserFunc != nil {
		return m.GetUserFunc(ctx, userID)
	}
	return nil, nil
}

func (m *Mock) GetGroup(ctx context.Context, groupID string) (*idp.GroupInfo, error) {
	if m.GetGroupFunc != nil {
		return m.GetGroupFunc(ctx, groupID)
	}
	return nil, nil
}

func (m *Mock) GetGroupByName(ctx context.Context, groupName string) (*string, error) {
	if m.GetGroupByNameFunc != nil {
		return m.GetGroupByNameFunc(ctx, groupName)
	}
	return nil, nil
}

func (m *Mock) CreateGroup(ctx context.Context, group *idp.Group) (*idp.GroupInfo, error) {
	if m.CreateGroupFunc != nil {
		return m.CreateGroupFunc(ctx, group)
	}
	return nil, nil
}

func (m *Mock) UpdateGroup(ctx context.Context, groupID string, group *idp.Group) (*idp.GroupInfo, error) {
	if m.UpdateGroupFunc != nil {
		return m.UpdateGroupFunc(ctx, groupID, group)
	}
	return nil, nil
}

func (m *Mock) AddMemberToGroup(ctx context.Context, groupID string, memberInfo *idp.GroupMember) error {
	if m.AddMemberToGroupFunc != nil {
		return m.AddMemberToGroupFunc(ctx, groupID, memberInfo)
	}
	return nil
}

func (m *Mock) CreateApplication(ctx context.Context, app *idp.Application) (*string, error) {
	if m.CreateApplicationFunc != nil {
		return m.CreateApplicationFunc(ctx, app)
	}
	appID := "mock-idp-app-id"
	return &appID, nil
}

func (m *Mock) DeleteApplication(ctx context.Context, applicationID string) error {
	if m.DeleteApplicationFunc != nil {
		return m.DeleteApplicationFunc(ctx, applicationID)
	}
	return nil
}

func (m *Mock) DeleteGroup(ctx context.Context, groupID string) error {
	if m.DeleteGroupFunc != nil {
		return m.DeleteGroupFunc(ctx, groupID)
	}
	return nil
}

func (m *Mock) GetApplicationInfo(ctx context.Context, applicationID string) (*idp.ApplicationInfo, error) {
	if m.GetApplicationInfoFunc != nil {
		return m.GetApplicationInfoFunc(ctx, applicationID)
	}
	return nil, nil
}

func (m *Mock) GetApplicationOIDC(ctx context.Context, applicationID string) (*idp.ApplicationOIDCInfo, error) {
	if m.GetApplicationOIDCFunc != nil {
		return m.GetApplicationOIDCFunc(ctx, applicationID)
	}
	return &idp.ApplicationOIDCInfo{
		ClientId:     "mock-client-id",
		ClientSecret: "mock-client-secret",
	}, nil
}
