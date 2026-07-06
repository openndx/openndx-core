package models

import (
	"reflect"
	"testing"
)

func TestAuthenticatedUser_GetPermissions_Caching(t *testing.T) {
	// Create test claims with admin role
	claims := &UserClaims{
		Email:     "admin@example.com",
		FirstName: "Admin",
		LastName:  "User",
		IdpUserID: "admin-123",
		Roles:     FlexibleStringSlice{"OpenDIF_Admin"},
		Groups:    []string{"admins"},
	}

	// Create authenticated user
	user, err := NewAuthenticatedUser(claims)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Get permissions multiple times
	permissions1 := user.GetPermissions()
	permissions2 := user.GetPermissions()
	permissions3 := user.GetPermissions()

	// Verify all calls return the same permissions
	if !reflect.DeepEqual(permissions1, permissions2) {
		t.Error("GetPermissions() should return consistent results")
	}

	if !reflect.DeepEqual(permissions2, permissions3) {
		t.Error("GetPermissions() should return consistent results")
	}

	// Verify permissions are correct for admin role
	expectedPermissions := RolePermissions[RoleAdmin]
	if len(permissions1) != len(expectedPermissions) {
		t.Errorf("Expected %d permissions for admin, got %d", len(expectedPermissions), len(permissions1))
	}

	// Convert to maps for easier comparison
	permissionMap := make(map[Permission]bool)
	for _, p := range permissions1 {
		permissionMap[p] = true
	}

	for _, expectedPerm := range expectedPermissions {
		if !permissionMap[expectedPerm] {
			t.Errorf("Expected permission %s not found in user permissions", expectedPerm)
		}
	}
}

func TestAuthenticatedUser_GetPermissions_Immutability(t *testing.T) {
	// Create test user with member role
	claims := &UserClaims{
		Email:     "member@example.com",
		IdpUserID: "member-123",
		Roles:     FlexibleStringSlice{"OpenDIF_Member"},
	}

	user, err := NewAuthenticatedUser(claims)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Get permissions and modify the returned slice
	permissions1 := user.GetPermissions()
	originalLength := len(permissions1)

	// Try to modify the returned slice
	if len(permissions1) > 0 {
		permissions1[0] = "modified_permission"
		_ = append(permissions1, "extra_permission")
	}

	// Get permissions again and verify they're unchanged
	permissions2 := user.GetPermissions()

	if len(permissions2) != originalLength {
		t.Error("GetPermissions() should return immutable results - length changed")
	}

	// Verify first permission wasn't modified (if there are permissions)
	if len(permissions2) > 0 && permissions2[0] == "modified_permission" {
		t.Error("GetPermissions() should return immutable results - content was modified")
	}
}

func TestAuthenticatedUser_MultipleRoles_PermissionMerging(t *testing.T) {
	// Create user with multiple roles
	claims := &UserClaims{
		Email:     "multirole@example.com",
		IdpUserID: "multi-123",
		Roles:     FlexibleStringSlice{"OpenDIF_Member", "OpenDIF_System"},
	}

	user, err := NewAuthenticatedUser(claims)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	permissions := user.GetPermissions()

	// Should have permissions from both roles
	memberPermissions := RolePermissions[RoleMember]
	systemPermissions := RolePermissions[RoleSystem]

	// Create expected merged permissions
	expectedPermissionSet := make(map[Permission]bool)
	for _, p := range memberPermissions {
		expectedPermissionSet[p] = true
	}
	for _, p := range systemPermissions {
		expectedPermissionSet[p] = true
	}

	// Verify user has all expected permissions
	userPermissionSet := make(map[Permission]bool)
	for _, p := range permissions {
		userPermissionSet[p] = true
	}

	if len(userPermissionSet) != len(expectedPermissionSet) {
		t.Errorf("Expected %d unique permissions, got %d", len(expectedPermissionSet), len(userPermissionSet))
	}

	for expectedPerm := range expectedPermissionSet {
		if !userPermissionSet[expectedPerm] {
			t.Errorf("Expected permission %s not found in merged permissions", expectedPerm)
		}
	}
}

func TestComputePermissions_EmptyRoles(t *testing.T) {
	permissions := computePermissions([]Role{})
	if len(permissions) != 0 {
		t.Errorf("Expected 0 permissions for empty roles, got %d", len(permissions))
	}
}

func TestComputePermissions_InvalidRoles(t *testing.T) {
	invalidRoles := []Role{"InvalidRole", "AnotherInvalidRole"}
	permissions := computePermissions(invalidRoles)
	if len(permissions) != 0 {
		t.Errorf("Expected 0 permissions for invalid roles, got %d", len(permissions))
	}
}

func TestComputePermissions_DuplicatePermissions(t *testing.T) {
	// This test assumes some roles might have overlapping permissions
	// Create a scenario where permissions might overlap
	allRoles := []Role{RoleAdmin, RoleMember, RoleSystem}
	permissions := computePermissions(allRoles)

	// Verify no duplicate permissions in the result
	permissionSet := make(map[Permission]bool)
	for _, p := range permissions {
		if permissionSet[p] {
			t.Errorf("Duplicate permission found: %s", p)
		}
		permissionSet[p] = true
	}

	// Verify the count matches the unique permissions
	if len(permissions) != len(permissionSet) {
		t.Errorf("Permission deduplication failed: got %d permissions, expected %d unique", len(permissions), len(permissionSet))
	}
}

// BenchmarkGetPermissions_Cached tests the performance of cached permissions
func BenchmarkGetPermissions_Cached(b *testing.B) {
	claims := &UserClaims{
		Email:     "admin@example.com",
		IdpUserID: "admin-123",
		Roles:     FlexibleStringSlice{"OpenDIF_Admin"},
	}

	user, err := NewAuthenticatedUser(claims)
	if err != nil {
		b.Fatalf("Failed to create user: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = user.GetPermissions()
	}
}

// BenchmarkGetPermissions_Uncached simulates the old implementation for comparison
func BenchmarkGetPermissions_Uncached(b *testing.B) {
	roles := []Role{RoleAdmin}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate the old GetPermissions implementation
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
		_ = permissions
	}
}

// BenchmarkUserCreation tests the performance impact of computing permissions during user creation
func BenchmarkUserCreation_WithPermissionCaching(b *testing.B) {
	claims := &UserClaims{
		Email:     "admin@example.com",
		IdpUserID: "admin-123",
		Roles:     FlexibleStringSlice{"OpenDIF_Admin"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewAuthenticatedUser(claims)
	}
}
