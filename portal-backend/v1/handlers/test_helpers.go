package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
	"github.com/OpenNDX/openndx-core/portal-backend/v1/utils"
)

// TestUser represents different test user personas for testing authorization scenarios.
// These users represent successfully authenticated users with different role assignments.
// For testing authentication failures (no user in context), use NewUnauthenticatedRequest instead.
type TestUser struct {
	IdpUserID string
	Email     string
	Roles     []models.Role
	User      *models.AuthenticatedUser
}

// Predefined test users with different roles for testing authorization scenarios
var (
	// AdminUser has full admin privileges
	AdminUser = TestUser{
		IdpUserID: "admin-test-user-123",
		Email:     "admin@test.com",
		Roles:     []models.Role{models.RoleAdmin},
	}

	// MemberUser has standard member privileges
	MemberUser = TestUser{
		IdpUserID: "member-test-user-456",
		Email:     "member@test.com",
		Roles:     []models.Role{models.RoleMember},
	}

	// SystemUser has system-level read access
	SystemUser = TestUser{
		IdpUserID: "system-test-user-789",
		Email:     "system@test.com",
		Roles:     []models.Role{models.RoleSystem},
	}
)

// init initializes the test users with their AuthenticatedUser instances
func init() {
	AdminUser.User = createTestUser(AdminUser.IdpUserID, AdminUser.Email, AdminUser.Roles)
	MemberUser.User = createTestUser(MemberUser.IdpUserID, MemberUser.Email, MemberUser.Roles)
	SystemUser.User = createTestUser(SystemUser.IdpUserID, SystemUser.Email, SystemUser.Roles)
}

// createTestUser creates an AuthenticatedUser from test data
func createTestUser(idpUserID, email string, roles []models.Role) *models.AuthenticatedUser {
	// Convert roles to string slice for UserClaims
	roleStrings := make([]string, len(roles))
	for i, role := range roles {
		roleStrings[i] = string(role)
	}

	claims := &models.UserClaims{
		IdpUserID: idpUserID,
		Email:     email,
		FirstName: "Test",
		LastName:  "User",
		Roles:     models.FlexibleStringSlice(roleStrings),
	}

	user, err := models.NewAuthenticatedUser(claims)
	if err != nil {
		panic(fmt.Sprintf("Failed to create test user: %v", err))
	}
	return user
}

// WithAuth creates a new HTTP request with the specified user authentication context
func WithAuth(req *http.Request, testUser TestUser) *http.Request {
	ctx := utils.SetAuthenticatedUser(req.Context(), testUser.User)
	return req.WithContext(ctx)
}

// WithAdminAuth is a convenience function to add admin authentication to a request
func WithAdminAuth(req *http.Request) *http.Request {
	return WithAuth(req, AdminUser)
}

// WithMemberAuth is a convenience function to add member authentication to a request
func WithMemberAuth(req *http.Request) *http.Request {
	return WithAuth(req, MemberUser)
}

// WithSystemAuth is a convenience function to add system authentication to a request
func WithSystemAuth(req *http.Request) *http.Request {
	return WithAuth(req, SystemUser)
}

// NewAuthenticatedRequest creates a new HTTP request with authentication context
func NewAuthenticatedRequest(method, url string, body io.Reader, testUser TestUser) *http.Request {
	req := httptest.NewRequest(method, url, body)
	return WithAuth(req, testUser)
}

// NewAdminRequest creates a new HTTP request with admin authentication
func NewAdminRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, AdminUser)
}

// NewMemberRequest creates a new HTTP request with member authentication
func NewMemberRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, MemberUser)
}

// NewSystemRequest creates a new HTTP request with system authentication
func NewSystemRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, SystemUser)
}

// NewUnauthenticatedRequest creates a new HTTP request without authentication context.
// Use this to test authentication failure scenarios where no authenticated user exists.
// This simulates a request from a user who hasn't provided valid JWT credentials.
func NewUnauthenticatedRequest(method, url string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, url, body)
	// Intentionally don't add authentication context to simulate authentication failure
	return req
}

// CreateCustomTestUser creates a test user with custom roles and permissions
func CreateCustomTestUser(idpUserID, email string, roles []models.Role) TestUser {
	user := createTestUser(idpUserID, email, roles)
	return TestUser{
		IdpUserID: idpUserID,
		Email:     email,
		Roles:     roles,
		User:      user,
	}
}
