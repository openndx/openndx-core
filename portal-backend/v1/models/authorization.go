package models

// AuthorizationMode defines how the system behaves when no explicit permission is defined for an endpoint
type AuthorizationMode string

const (
	// AuthorizationModeFailClosed - Deny all access to undefined endpoints (most secure)
	AuthorizationModeFailClosed AuthorizationMode = "fail_closed"

	// AuthorizationModeFailOpenAdminSystem - Allow admin and system users, deny others (current behavior)
	AuthorizationModeFailOpenAdminSystem AuthorizationMode = "fail_open_admin_system"

	// AuthorizationModeFailOpenAdmin - Allow only admin users, deny others
	AuthorizationModeFailOpenAdmin AuthorizationMode = "fail_open_admin"
)

// Role represents user roles in the system
type Role string

const (
	RoleAdmin  Role = "OpenDIF_Admin"  // Full access to all resources
	RoleMember Role = "OpenDIF_Member" // Access to own resources and public endpoints
	RoleSystem Role = "OpenDIF_System" // System-level access for internal services
)

// Permission represents specific permissions
type Permission string

const (
	// Schema permissions
	PermissionCreateSchema   Permission = "schema:create"
	PermissionReadSchema     Permission = "schema:read"
	PermissionUpdateSchema   Permission = "schema:update"
	PermissionDeleteSchema   Permission = "schema:delete"
	PermissionReadAllSchemas Permission = "schema:read:all"

	// Schema submission permissions
	PermissionCreateSchemaSubmission   Permission = "schema_submission:create"
	PermissionReadSchemaSubmission     Permission = "schema_submission:read"
	PermissionUpdateSchemaSubmission   Permission = "schema_submission:update"
	PermissionDeleteSchemaSubmission   Permission = "schema_submission:delete"
	PermissionReadAllSchemaSubmissions Permission = "schema_submission:read:all"
	PermissionApproveSchemaSubmission  Permission = "schema_submission:approve"

	// Application permissions
	PermissionCreateApplication   Permission = "application:create"
	PermissionReadApplication     Permission = "application:read"
	PermissionUpdateApplication   Permission = "application:update"
	PermissionDeleteApplication   Permission = "application:delete"
	PermissionReadAllApplications Permission = "application:read:all"

	// Application submission permissions
	PermissionCreateApplicationSubmission   Permission = "application_submission:create"
	PermissionReadApplicationSubmission     Permission = "application_submission:read"
	PermissionUpdateApplicationSubmission   Permission = "application_submission:update"
	PermissionDeleteApplicationSubmission   Permission = "application_submission:delete"
	PermissionReadAllApplicationSubmissions Permission = "application_submission:read:all"
	PermissionApproveApplicationSubmission  Permission = "application_submission:approve"

	// Member permissions
	PermissionCreateMember   Permission = "member:create"
	PermissionReadMember     Permission = "member:read"
	PermissionUpdateMember   Permission = "member:update"
	PermissionDeleteMember   Permission = "member:delete"
	PermissionReadAllMembers Permission = "member:read:all"
)

// RolePermissions defines what permissions each role has
var RolePermissions = map[Role][]Permission{
	RoleAdmin: {
		// Admin has all permissions
		PermissionCreateSchema, PermissionReadSchema, PermissionUpdateSchema, PermissionDeleteSchema, PermissionReadAllSchemas,
		PermissionCreateSchemaSubmission, PermissionReadSchemaSubmission, PermissionUpdateSchemaSubmission,
		PermissionDeleteSchemaSubmission, PermissionReadAllSchemaSubmissions, PermissionApproveSchemaSubmission,
		PermissionCreateApplication, PermissionReadApplication, PermissionUpdateApplication, PermissionDeleteApplication,
		PermissionReadAllApplications, PermissionCreateApplicationSubmission, PermissionReadApplicationSubmission,
		PermissionUpdateApplicationSubmission, PermissionDeleteApplicationSubmission, PermissionReadAllApplicationSubmissions,
		PermissionApproveApplicationSubmission, PermissionCreateMember, PermissionReadMember, PermissionUpdateMember,
		PermissionDeleteMember, PermissionReadAllMembers,
	},
	RoleMember: {
		// Members can create, read, and update their own resources
		PermissionCreateSchema, PermissionReadSchema, PermissionUpdateSchema,
		PermissionCreateSchemaSubmission, PermissionReadSchemaSubmission, PermissionUpdateSchemaSubmission,
		PermissionCreateApplication, PermissionReadApplication, PermissionUpdateApplication,
		PermissionCreateApplicationSubmission, PermissionReadApplicationSubmission, PermissionUpdateApplicationSubmission,
		PermissionReadMember, PermissionUpdateMember,
	},
	RoleSystem: {
		// System role has broad read access for internal services
		PermissionReadSchema, PermissionReadAllSchemas,
		PermissionReadSchemaSubmission, PermissionReadAllSchemaSubmissions,
		PermissionReadApplication, PermissionReadAllApplications,
		PermissionReadApplicationSubmission, PermissionReadAllApplicationSubmissions,
		PermissionReadMember, PermissionReadAllMembers,
	},
}

// EndpointPermission defines the required permission for each endpoint
type EndpointPermission struct {
	Method              string
	Path                string
	Permission          Permission
	IsOwnershipRequired bool // Whether the user must own the resource
}

// EndpointPermissions maps HTTP endpoints to required permissions
var EndpointPermissions = []EndpointPermission{
	// Schema endpoints
	{"GET", "/api/v1/schemas", PermissionReadSchema, false},
	{"POST", "/api/v1/schemas", PermissionCreateSchema, false},
	{"GET", "/api/v1/schemas/*", PermissionReadSchema, true},
	{"PUT", "/api/v1/schemas/*", PermissionUpdateSchema, true},
	{"DELETE", "/api/v1/schemas/*", PermissionDeleteSchema, true},

	// Schema submission endpoints
	{"GET", "/api/v1/schema-submissions", PermissionReadSchemaSubmission, false},
	{"POST", "/api/v1/schema-submissions", PermissionCreateSchemaSubmission, false},
	{"GET", "/api/v1/schema-submissions/*", PermissionReadSchemaSubmission, true},
	{"PUT", "/api/v1/schema-submissions/*", PermissionUpdateSchemaSubmission, true},

	// Application endpoints
	{"GET", "/api/v1/applications", PermissionReadApplication, false},
	{"POST", "/api/v1/applications", PermissionCreateApplication, false},
	{"GET", "/api/v1/applications/*", PermissionReadApplication, true},
	{"PUT", "/api/v1/applications/*", PermissionUpdateApplication, true},
	{"DELETE", "/api/v1/applications/*", PermissionDeleteApplication, true},

	// Application submission endpoints
	{"GET", "/api/v1/application-submissions", PermissionReadApplicationSubmission, false},
	{"POST", "/api/v1/application-submissions", PermissionCreateApplicationSubmission, false},
	{"GET", "/api/v1/application-submissions/*", PermissionReadApplicationSubmission, true},
	{"PUT", "/api/v1/application-submissions/*", PermissionUpdateApplicationSubmission, true},

	// Member endpoints
	{"GET", "/api/v1/members", PermissionReadMember, false},
	{"POST", "/api/v1/members", PermissionCreateMember, false},
	{"GET", "/api/v1/members/*", PermissionReadMember, true},
	{"PUT", "/api/v1/members/*", PermissionUpdateMember, true},
}

// HasPermission checks if a role has a specific permission
func (r Role) HasPermission(permission Permission) bool {
	permissions, exists := RolePermissions[r]
	if !exists {
		return false
	}

	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// String returns the string representation of the role
func (r Role) String() string {
	return string(r)
}

// IsValid checks if the role is valid
func (r Role) IsValid() bool {
	_, exists := RolePermissions[r]
	return exists
}
