package models

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// UserClaims represents the JWT claims for a user
type UserClaims struct {
	Email       string              `json:"email"`
	FirstName   string              `json:"given_name"`
	LastName    string              `json:"family_name"`
	PhoneNumber string              `json:"phone_number"`
	Roles       FlexibleStringSlice `json:"roles"`
	Groups      FlexibleStringSlice `json:"groups"`
	IdpUserID   string              `json:"sub"` // Subject is typically the user ID from IdP
	// Standard JWT claims - using int64 for Unix timestamps
	Issuer    string              `json:"iss"`
	Audience  FlexibleStringSlice `json:"aud"`
	ExpiresAt int64               `json:"exp"`
	IssuedAt  int64               `json:"iat"`
	NotBefore int64               `json:"nbf"`
}

// GetExpirationTime implements jwt.Claims interface
func (c *UserClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.ExpiresAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.ExpiresAt, 0)), nil
}

// GetIssuedAt implements jwt.Claims interface
func (c *UserClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	if c.IssuedAt == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.IssuedAt, 0)), nil
}

// GetNotBefore implements jwt.Claims interface
func (c *UserClaims) GetNotBefore() (*jwt.NumericDate, error) {
	if c.NotBefore == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.NotBefore, 0)), nil
}

// GetIssuer implements jwt.Claims interface
func (c *UserClaims) GetIssuer() (string, error) {
	return c.Issuer, nil
}

// GetSubject implements jwt.Claims interface
func (c *UserClaims) GetSubject() (string, error) {
	return c.IdpUserID, nil
}

// GetAudience implements jwt.Claims interface
func (c *UserClaims) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings(c.Audience.ToStringSlice()), nil
}

// AuthenticatedUser represents the authenticated user context
type AuthenticatedUser struct {
	IdpUserID   string    `json:"idpUserId"`
	Email       string    `json:"email"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	PhoneNumber string    `json:"phoneNumber"`
	Roles       []Role    `json:"roles"`
	Groups      []string  `json:"groups"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`

	// Cached permissions - computed once during user creation for performance
	permissions []Permission `json:"-"` // Don't expose in JSON, use GetPermissions() method

	// Cached member ID - populated on first access to avoid repeated database queries
	// Protected by mutex for thread safety
	memberIDMutex sync.RWMutex `json:"-"` // Protects memberID and memberIDError fields
	memberID      string       `json:"-"` // Don't expose in JSON
	memberIDError error        `json:"-"` // Cache the error state as well
}

// AuthContext represents the authentication context in HTTP requests
type AuthContext struct {
	User        *AuthenticatedUser `json:"user"`
	Token       string             `json:"-"` // Don't expose in JSON
	IssuedBy    string             `json:"issuedBy"`
	Audience    []string           `json:"audience"`
	Permissions []Permission       `json:"permissions"`
}

// HasRole checks if the user has a specific role
func (u *AuthenticatedUser) HasRole(role Role) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the user has any of the specified roles
func (u *AuthenticatedUser) HasAnyRole(roles ...Role) bool {
	for _, requiredRole := range roles {
		for _, userRole := range u.Roles {
			if userRole == requiredRole {
				return true
			}
		}
	}
	return false
}

// HasPermission checks if the user has a specific permission based on their roles
func (u *AuthenticatedUser) HasPermission(permission Permission) bool {
	for _, role := range u.Roles {
		if role.HasPermission(permission) {
			return true
		}
	}
	return false
}

// IsAdmin checks if the user has admin role
func (u *AuthenticatedUser) IsAdmin() bool {
	return u.HasRole(RoleAdmin)
}

// IsMember checks if the user has member role
func (u *AuthenticatedUser) IsMember() bool {
	return u.HasRole(RoleMember)
}

// IsSystem checks if the user has system role
func (u *AuthenticatedUser) IsSystem() bool {
	return u.HasRole(RoleSystem)
}

// GetPrimaryRole returns the highest priority role (Admin > System > Member)
func (u *AuthenticatedUser) GetPrimaryRole() Role {
	if u.HasRole(RoleAdmin) {
		return RoleAdmin
	}
	if u.HasRole(RoleSystem) {
		return RoleSystem
	}
	if u.HasRole(RoleMember) {
		return RoleMember
	}
	return RoleMember // Default to member if no roles found
}

// GetPermissions returns all permissions the user has based on their roles
// Uses cached permissions computed during user creation for optimal performance
func (u *AuthenticatedUser) GetPermissions() []Permission {
	// Return a copy of the cached permissions to prevent external modification
	result := make([]Permission, len(u.permissions))
	copy(result, u.permissions)
	return result
}

// IsTokenExpired checks if the user's token is expired
func (u *AuthenticatedUser) IsTokenExpired() bool {
	return time.Now().After(u.ExpiresAt)
}

// GetCachedMemberID returns the cached member ID if available
// Thread-safe using read lock
func (u *AuthenticatedUser) GetCachedMemberID() (string, bool) {
	u.memberIDMutex.RLock()
	defer u.memberIDMutex.RUnlock()
	return u.memberID, u.memberID != ""
}

// SetCachedMemberID sets the cached member ID and error state
// Thread-safe using write lock
func (u *AuthenticatedUser) SetCachedMemberID(memberID string, err error) {
	u.memberIDMutex.Lock()
	defer u.memberIDMutex.Unlock()
	u.memberID = memberID
	u.memberIDError = err
}

// GetCachedMemberIDError returns the cached error from member ID lookup
// Thread-safe using read lock
func (u *AuthenticatedUser) GetCachedMemberIDError() error {
	u.memberIDMutex.RLock()
	defer u.memberIDMutex.RUnlock()
	return u.memberIDError
}

// GetCachedMemberIDWithError atomically returns both the cached member ID and error state
// This ensures consistency when reading both values together
func (u *AuthenticatedUser) GetCachedMemberIDWithError() (memberID string, cached bool, err error) {
	u.memberIDMutex.RLock()
	defer u.memberIDMutex.RUnlock()
	return u.memberID, u.memberID != "", u.memberIDError
}

// computePermissions calculates all permissions for the given roles
func computePermissions(roles []Role) []Permission {
	permissionSet := make(map[Permission]bool)

	for _, role := range roles {
		if permissions, exists := RolePermissions[role]; exists {
			for _, permission := range permissions {
				permissionSet[permission] = true
			}
		}
	}

	var permissions []Permission
	for permission := range permissionSet {
		permissions = append(permissions, permission)
	}

	return permissions
}

// NewAuthenticatedUser creates a new authenticated user from JWT claims
// Returns an error if no valid roles are found in the claims
func NewAuthenticatedUser(claims *UserClaims) (*AuthenticatedUser, error) {
	// Convert string roles to Role type, tracking invalid ones for security logging
	var roles []Role
	var invalidRoles []string
	for _, roleStr := range claims.Roles.ToStringSlice() {
		role := Role(roleStr)
		if role.IsValid() {
			roles = append(roles, role)
		} else {
			invalidRoles = append(invalidRoles, roleStr)
		}
	}

	// Log security-relevant event when invalid roles are filtered
	if len(invalidRoles) > 0 {
		slog.Warn("Invalid roles filtered from JWT claims", "user", claims.IdpUserID, "invalid_roles", invalidRoles)
	}

	// If no valid roles found, deny access for security
	if len(roles) == 0 {
		return nil, fmt.Errorf("access denied: no valid roles found in JWT claims for user %s", claims.IdpUserID)
	}

	// Compute permissions once during user creation for optimal performance
	permissions := computePermissions(roles)

	// Convert Unix timestamps to time.Time for AuthenticatedUser
	var issuedAt, expiresAt time.Time
	if claims.IssuedAt != 0 {
		issuedAt = time.Unix(claims.IssuedAt, 0)
	}
	if claims.ExpiresAt != 0 {
		expiresAt = time.Unix(claims.ExpiresAt, 0)
	}

	return &AuthenticatedUser{
		IdpUserID:   claims.IdpUserID,
		Email:       claims.Email,
		FirstName:   claims.FirstName,
		LastName:    claims.LastName,
		PhoneNumber: claims.PhoneNumber,
		Roles:       roles,
		Groups:      claims.Groups.ToStringSlice(),
		IssuedAt:    issuedAt,
		ExpiresAt:   expiresAt,
		permissions: permissions,
	}, nil
}
