// Package callertest provides authenticated test personas and request helpers
// for Portal Backend handler tests. Import it only from _test.go files.
package callertest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/authn/authntest"
	"github.com/openndx/openndx-core/internal/pb/caller"
)

// TestUser is an authenticated caller with the scopes its token was granted
// and, when MemberID is set, the member caller.Middleware would have found
// for its organization unit. For testing authentication failures (no caller
// in context), use NewUnauthenticatedRequest instead.
type TestUser struct {
	Subject  string
	ClientID string
	OuID     string
	Email    string
	Scopes   []string
	MemberID string
}

// WithMember returns a copy of u whose organization unit belongs to memberID.
func (u TestUser) WithMember(memberID string) TestUser {
	u.MemberID = memberID
	return u
}

// MemberScopes are the scopes the OpenNDX_Member role grants: read and write
// within the member's own organization unit.
var MemberScopes = []string{
	caller.ScopeMemberRead, caller.ScopeMemberWrite,
	caller.ScopeSchemaRead, caller.ScopeSchemaWrite,
	caller.ScopeSchemaSubmissionRead, caller.ScopeSchemaSubmissionWrite,
	caller.ScopeApplicationRead, caller.ScopeApplicationWrite,
	caller.ScopeApplicationSubmissionRead, caller.ScopeApplicationSubmissionWrite,
}

// ReadAllScopes are read access across every organization unit and nothing
// else. Each -all scope comes with its base scope, because -all alone never
// opens a route.
var ReadAllScopes = []string{
	caller.ScopeMemberRead, caller.ScopeMemberReadAll,
	caller.ScopeSchemaRead, caller.ScopeSchemaReadAll,
	caller.ScopeSchemaSubmissionRead, caller.ScopeSchemaSubmissionReadAll,
	caller.ScopeApplicationRead, caller.ScopeApplicationReadAll,
	caller.ScopeApplicationSubmissionRead, caller.ScopeApplicationSubmissionReadAll,
}

// AdminScopes are the scopes the OpenNDX_Admin role grants: every PB scope.
var AdminScopes = caller.AllScopes

// Predefined test users for testing authorization scenarios. None has a
// member; use WithMember for a caller whose organization unit has one.
var (
	// AdminUser holds every PB scope
	AdminUser = TestUser{
		Subject:  "admin-test-user-123",
		ClientID: "admin-test-client",
		OuID:     "admin-test-ou-123",
		Email:    "admin@test.com",
		Scopes:   AdminScopes,
	}

	// MemberUser holds the own-organization-unit scopes
	MemberUser = TestUser{
		Subject:  "member-test-user-456",
		ClientID: "member-test-client",
		OuID:     "member-test-ou-456",
		Email:    "member@test.com",
		Scopes:   MemberScopes,
	}

	// ReadOnlyUser can read across organization units but write nothing
	ReadOnlyUser = TestUser{
		Subject:  "readonly-test-user-789",
		ClientID: "readonly-test-client",
		OuID:     "readonly-test-ou-789",
		Email:    "readonly@test.com",
		Scopes:   ReadAllScopes,
	}
)

// WithAuth returns req carrying testUser as the principal internal/authn
// would have validated, and its member if it has one.
func WithAuth(req *http.Request, testUser TestUser) *http.Request {
	claims := map[string]any{
		"sub": testUser.Subject, "ouId": testUser.OuID,
		"email": testUser.Email, "scope": testUser.Scopes,
	}
	if testUser.ClientID != "" {
		claims["client_id"] = testUser.ClientID
	}
	ctx := context.WithValue(req.Context(), authn.AuthContextKey, &authn.AuthContext{
		Principal: authntest.MustPrincipal(claims),
	})
	if testUser.MemberID != "" {
		ctx = caller.WithMemberID(ctx, testUser.MemberID)
	}
	return req.WithContext(ctx)
}

// NewAuthenticatedRequest creates a new HTTP request with authentication context
func NewAuthenticatedRequest(method, url string, body io.Reader, testUser TestUser) *http.Request {
	req := httptest.NewRequest(method, url, body)
	return WithAuth(req, testUser)
}

// NewAdminRequest creates a new HTTP request authenticated as AdminUser
func NewAdminRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, AdminUser)
}

// NewMemberRequest creates a new HTTP request authenticated as MemberUser
func NewMemberRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, MemberUser)
}

// NewReadOnlyRequest creates a new HTTP request authenticated as ReadOnlyUser
func NewReadOnlyRequest(method, url string, body io.Reader) *http.Request {
	return NewAuthenticatedRequest(method, url, body, ReadOnlyUser)
}

// NewUnauthenticatedRequest creates a new HTTP request without authentication context.
// Use this to test authentication failure scenarios where no authenticated user exists.
// This simulates a request from a user who hasn't provided valid JWT credentials.
func NewUnauthenticatedRequest(method, url string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, url, body)
	// Intentionally don't add authentication context to simulate authentication failure
	return req
}

// CreateCustomTestUser creates a test user in the given organization unit
// with the given scopes
func CreateCustomTestUser(subject, email, ouID string, scopes []string) TestUser {
	return TestUser{
		Subject: subject,
		OuID:    ouID,
		Email:   email,
		Scopes:  scopes,
	}
}
