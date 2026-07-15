package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRole_HasPermission(t *testing.T) {
	tests := []struct {
		name       string
		role       Role
		permission Permission
		want       bool
	}{
		{
			name:       "Admin has CreateSchema",
			role:       RoleAdmin,
			permission: PermissionCreateSchema,
			want:       true,
		},
		{
			name:       "Member has CreateSchema",
			role:       RoleMember,
			permission: PermissionCreateSchema,
			want:       true,
		},
		{
			name:       "System has ReadSchema",
			role:       RoleSystem,
			permission: PermissionReadSchema,
			want:       true,
		},
		{
			name:       "Member does not have DeleteMember",
			role:       RoleMember,
			permission: PermissionDeleteMember,
			want:       false,
		},
		{
			name:       "Invalid role has no permissions",
			role:       Role("invalid"),
			permission: PermissionReadSchema,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.role.HasPermission(tt.permission))
		})
	}
}

func TestRole_IsValid(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want bool
	}{
		{"Admin", RoleAdmin, true},
		{"Member", RoleMember, true},
		{"System", RoleSystem, true},
		{"Invalid", Role("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.role.IsValid())
		})
	}
}

func TestRole_String(t *testing.T) {
	assert.Equal(t, "OpenDIF_Admin", RoleAdmin.String())
	assert.Equal(t, "OpenDIF_Member", RoleMember.String())
}
