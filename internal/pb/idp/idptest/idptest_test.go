package idptest

import (
	"context"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/idp"
)

// TestMock_CallsConfiguredFuncs checks that every Mock method delegates to its
// matching Func field when one is set.
func TestMock_CallsConfiguredFuncs(t *testing.T) {
	ctx := context.Background()
	called := map[string]bool{}
	mark := func(name string) { called[name] = true }

	m := &Mock{
		CreateUserFunc: func(context.Context, *idp.User) (*idp.UserInfo, error) {
			mark("CreateUser")
			return nil, nil
		},
		UpdateUserFunc: func(context.Context, string, *idp.User) (*idp.UserInfo, error) {
			mark("UpdateUser")
			return nil, nil
		},
		DeleteUserFunc: func(context.Context, string) error {
			mark("DeleteUser")
			return nil
		},
		AddMemberToGroupByGroupNameFunc: func(context.Context, string, *idp.GroupMember) (*string, error) {
			mark("AddMemberToGroupByGroupName")
			return nil, nil
		},
		RemoveMemberFromGroupFunc: func(context.Context, string, string) error {
			mark("RemoveMemberFromGroup")
			return nil
		},
		GetUserFunc: func(context.Context, string) (*idp.UserInfo, error) {
			mark("GetUser")
			return nil, nil
		},
		GetGroupFunc: func(context.Context, string) (*idp.GroupInfo, error) {
			mark("GetGroup")
			return nil, nil
		},
		GetGroupByNameFunc: func(context.Context, string) (*string, error) {
			mark("GetGroupByName")
			return nil, nil
		},
		CreateGroupFunc: func(context.Context, *idp.Group) (*idp.GroupInfo, error) {
			mark("CreateGroup")
			return nil, nil
		},
		UpdateGroupFunc: func(context.Context, string, *idp.Group) (*idp.GroupInfo, error) {
			mark("UpdateGroup")
			return nil, nil
		},
		AddMemberToGroupFunc: func(context.Context, string, *idp.GroupMember) error {
			mark("AddMemberToGroup")
			return nil
		},
		CreateApplicationFunc: func(context.Context, *idp.Application) (*string, error) {
			mark("CreateApplication")
			return nil, nil
		},
		DeleteApplicationFunc: func(context.Context, string) error {
			mark("DeleteApplication")
			return nil
		},
		DeleteGroupFunc: func(context.Context, string) error {
			mark("DeleteGroup")
			return nil
		},
		GetApplicationInfoFunc: func(context.Context, string) (*idp.ApplicationInfo, error) {
			mark("GetApplicationInfo")
			return nil, nil
		},
		GetApplicationOIDCFunc: func(context.Context, string) (*idp.ApplicationOIDCInfo, error) {
			mark("GetApplicationOIDC")
			return nil, nil
		},
	}

	_, _ = m.CreateUser(ctx, &idp.User{})
	_, _ = m.UpdateUser(ctx, "u", &idp.User{})
	_ = m.DeleteUser(ctx, "u")
	_, _ = m.AddMemberToGroupByGroupName(ctx, "g", &idp.GroupMember{})
	_ = m.RemoveMemberFromGroup(ctx, "g", "u")
	_, _ = m.GetUser(ctx, "u")
	_, _ = m.GetGroup(ctx, "g")
	_, _ = m.GetGroupByName(ctx, "g")
	_, _ = m.CreateGroup(ctx, &idp.Group{})
	_, _ = m.UpdateGroup(ctx, "g", &idp.Group{})
	_ = m.AddMemberToGroup(ctx, "g", &idp.GroupMember{})
	_, _ = m.CreateApplication(ctx, &idp.Application{})
	_ = m.DeleteApplication(ctx, "a")
	_ = m.DeleteGroup(ctx, "g")
	_, _ = m.GetApplicationInfo(ctx, "a")
	_, _ = m.GetApplicationOIDC(ctx, "a")

	for _, name := range []string{
		"CreateUser", "UpdateUser", "DeleteUser", "AddMemberToGroupByGroupName",
		"RemoveMemberFromGroup", "GetUser", "GetGroup", "GetGroupByName",
		"CreateGroup", "UpdateGroup", "AddMemberToGroup", "CreateApplication",
		"DeleteApplication", "DeleteGroup", "GetApplicationInfo", "GetApplicationOIDC",
	} {
		if !called[name] {
			t.Errorf("%s did not call its configured Func", name)
		}
	}
}
