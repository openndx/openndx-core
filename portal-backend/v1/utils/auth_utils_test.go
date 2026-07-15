package utils

import (
	"testing"

	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
)

func TestFindEndpointPermission(t *testing.T) {
	// Reset cache before each test to ensure clean state
	ResetEndpointCacheForTesting()

	tests := []struct {
		name          string
		method        string
		path          string
		expectedFound bool
		expectedPerm  models.Permission
		expectedOwner bool
	}{
		{
			name:          "Exact match - GET schemas collection",
			method:        "GET",
			path:          "/api/v1/schemas",
			expectedFound: true,
			expectedPerm:  models.PermissionReadSchema,
			expectedOwner: false,
		},
		{
			name:          "Exact match - POST members",
			method:        "POST",
			path:          "/api/v1/members",
			expectedFound: true,
			expectedPerm:  models.PermissionCreateMember,
			expectedOwner: false,
		},
		{
			name:          "Wildcard match - GET specific schema",
			method:        "GET",
			path:          "/api/v1/schemas/12345",
			expectedFound: true,
			expectedPerm:  models.PermissionReadSchema,
			expectedOwner: true,
		},
		{
			name:          "Wildcard match - PUT specific application",
			method:        "PUT",
			path:          "/api/v1/applications/abcd-efgh",
			expectedFound: true,
			expectedPerm:  models.PermissionUpdateApplication,
			expectedOwner: true,
		},
		{
			name:          "No match - unknown endpoint",
			method:        "GET",
			path:          "/api/v1/unknown",
			expectedFound: false,
		},
		{
			name:          "No match - wrong method",
			method:        "PATCH",
			path:          "/api/v1/schemas",
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep, found := FindEndpointPermission(tt.method, tt.path)

			if found != tt.expectedFound {
				t.Errorf("FindEndpointPermission() found = %v, want %v", found, tt.expectedFound)
				return
			}

			if tt.expectedFound {
				if ep == nil {
					t.Errorf("FindEndpointPermission() returned nil endpoint permission when found = true")
					return
				}

				if ep.Permission != tt.expectedPerm {
					t.Errorf("FindEndpointPermission() permission = %v, want %v", ep.Permission, tt.expectedPerm)
				}

				if ep.IsOwnershipRequired != tt.expectedOwner {
					t.Errorf("FindEndpointPermission() ownership required = %v, want %v", ep.IsOwnershipRequired, tt.expectedOwner)
				}
			}
		})
	}
}

func TestFindEndpointPermissionCacheInitialization(t *testing.T) {
	// Reset cache
	ResetEndpointCacheForTesting()

	// First call should initialize cache
	_, found1 := FindEndpointPermission("GET", "/api/v1/schemas")
	if !found1 {
		t.Error("Expected to find GET /api/v1/schemas endpoint")
	}

	// Cache should now be initialized
	if endpointCache == nil {
		t.Error("Expected endpoint cache to be initialized after first call")
	}

	// Verify cache contains expected data
	if len(endpointCache.exactMatches) == 0 {
		t.Error("Expected exact matches cache to contain entries")
	}

	// Second call should use cached data
	_, found2 := FindEndpointPermission("GET", "/api/v1/schemas")
	if !found2 {
		t.Error("Expected to find GET /api/v1/schemas endpoint on cached lookup")
	}
}

func BenchmarkFindEndpointPermission(b *testing.B) {
	// Reset cache to test initialization
	endpointCache = nil

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Test both exact match and wildcard pattern
		FindEndpointPermission("GET", "/api/v1/schemas")
		FindEndpointPermission("GET", "/api/v1/schemas/12345")
	}
}

func BenchmarkFindEndpointPermissionCached(b *testing.B) {
	// Pre-initialize cache
	endpointCache = nil
	FindEndpointPermission("GET", "/api/v1/schemas")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Test both exact match and wildcard pattern with warm cache
		FindEndpointPermission("GET", "/api/v1/schemas")
		FindEndpointPermission("GET", "/api/v1/schemas/12345")
	}
}
