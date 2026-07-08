package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserClaims_Methods(t *testing.T) {
	now := time.Now()
	claims := &UserClaims{
		Issuer:    "test-issuer",
		IdpUserID: "test-subject",
		Audience:  FlexibleStringSlice{"test-audience"},
		ExpiresAt: now.Add(time.Hour).Unix(),
		IssuedAt:  now.Unix(),
		NotBefore: now.Add(-time.Hour).Unix(),
	}

	// Test GetExpirationTime
	exp, err := claims.GetExpirationTime()
	assert.NoError(t, err)
	assert.Equal(t, claims.ExpiresAt, exp.Time.Unix())

	// Test GetIssuedAt
	iat, err := claims.GetIssuedAt()
	assert.NoError(t, err)
	assert.Equal(t, claims.IssuedAt, iat.Time.Unix())

	// Test GetNotBefore
	nbf, err := claims.GetNotBefore()
	assert.NoError(t, err)
	assert.Equal(t, claims.NotBefore, nbf.Time.Unix())

	// Test GetIssuer
	iss, err := claims.GetIssuer()
	assert.NoError(t, err)
	assert.Equal(t, claims.Issuer, iss)

	// Test GetSubject
	sub, err := claims.GetSubject()
	assert.NoError(t, err)
	assert.Equal(t, claims.IdpUserID, sub)

	// Test GetAudience
	aud, err := claims.GetAudience()
	assert.NoError(t, err)
	assert.Equal(t, []string{"test-audience"}, []string(aud))

	// Test zero values
	emptyClaims := &UserClaims{}
	exp, _ = emptyClaims.GetExpirationTime()
	assert.Nil(t, exp)
	iat, _ = emptyClaims.GetIssuedAt()
	assert.Nil(t, iat)
	nbf, _ = emptyClaims.GetNotBefore()
	assert.Nil(t, nbf)
}

func TestAuthenticatedUser_Methods(t *testing.T) {
	user := &AuthenticatedUser{
		Roles: []Role{RoleAdmin, RoleMember},
		permissions: []Permission{
			PermissionCreateSchema,
			PermissionReadSchema,
		},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	// Test HasRole
	assert.True(t, user.HasRole(RoleAdmin))
	assert.True(t, user.HasRole(RoleMember))
	assert.False(t, user.HasRole(RoleSystem))

	// Test HasAnyRole
	assert.True(t, user.HasAnyRole(RoleAdmin, RoleSystem))
	assert.True(t, user.HasAnyRole(RoleSystem, RoleMember))
	assert.False(t, user.HasAnyRole(RoleSystem))

	// Test HasPermission
	assert.True(t, user.HasPermission(PermissionCreateSchema))

	userMemberOnly := &AuthenticatedUser{Roles: []Role{RoleMember}}
	assert.False(t, userMemberOnly.HasPermission(PermissionDeleteMember))

	// Test IsAdmin/IsMember/IsSystem
	assert.True(t, user.IsAdmin())
	assert.True(t, user.IsMember())
	assert.False(t, user.IsSystem())

	// Test GetPrimaryRole
	assert.Equal(t, RoleAdmin, user.GetPrimaryRole())

	userMember := &AuthenticatedUser{Roles: []Role{RoleMember}}
	assert.Equal(t, RoleMember, userMember.GetPrimaryRole())

	userSystem := &AuthenticatedUser{Roles: []Role{RoleSystem}}
	assert.Equal(t, RoleSystem, userSystem.GetPrimaryRole())

	// Test GetPermissions
	perms := user.GetPermissions()
	assert.Len(t, perms, 2)
	assert.Contains(t, perms, PermissionCreateSchema)

	// Test IsTokenExpired
	assert.False(t, user.IsTokenExpired())

	expiredUser := &AuthenticatedUser{ExpiresAt: time.Now().Add(-time.Hour)}
	assert.True(t, expiredUser.IsTokenExpired())
}

func TestAuthenticatedUser_CachedMemberID(t *testing.T) {
	user := &AuthenticatedUser{}

	// Initial state
	id, cached := user.GetCachedMemberID()
	assert.False(t, cached)
	assert.Empty(t, id)

	// Set cached ID
	user.SetCachedMemberID("mem-123", nil)

	// Check cached ID
	id, cached = user.GetCachedMemberID()
	assert.True(t, cached)
	assert.Equal(t, "mem-123", id)
	assert.NoError(t, user.GetCachedMemberIDError())

	// Check GetCachedMemberIDWithError
	id, cached, err := user.GetCachedMemberIDWithError()
	assert.True(t, cached)
	assert.Equal(t, "mem-123", id)
	assert.NoError(t, err)
}
