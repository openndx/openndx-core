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

// ApplicationManager defines a contract for all Identity Providers
type ApplicationManager interface {
	GetApplicationInfo(ctx context.Context, applicationId string) (*ApplicationInfo, error)
	CreateApplication(ctx context.Context, app *Application) (*string, error)
	GetApplicationOIDC(ctx context.Context, applicationId string) (*ApplicationOIDCInfo, error)
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
