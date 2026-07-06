package handlers

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gov-dx-sandbox/portal-backend/v1/models"
	"github.com/gov-dx-sandbox/portal-backend/v1/services"
	"github.com/stretchr/testify/assert"
)

func TestNewV1Handler_MissingEnvVars(t *testing.T) {
	// Save current env vars
	originalBaseURL := os.Getenv("ASGARDEO_BASE_URL")
	originalClientID := os.Getenv("ASGARDEO_CLIENT_ID")
	originalClientSecret := os.Getenv("ASGARDEO_CLIENT_SECRET")
	originalJWKS := os.Getenv("ASGARDEO_JWKS_URL")
	originalIssuer := os.Getenv("ASGARDEO_ISSUER")
	originalTokenURL := os.Getenv("ASGARDEO_TOKEN_URL")
	originalPDPURLStd := os.Getenv("PDP_SERVICEURL")
	originalPDPKey := os.Getenv("CHOREO_PDP_CONNECTION_CHOREOAPIKEY")

	// Restore env vars after test
	defer func() {
		os.Setenv("ASGARDEO_BASE_URL", originalBaseURL)
		os.Setenv("ASGARDEO_CLIENT_ID", originalClientID)
		os.Setenv("ASGARDEO_CLIENT_SECRET", originalClientSecret)
		os.Setenv("ASGARDEO_JWKS_URL", originalJWKS)
		os.Setenv("ASGARDEO_ISSUER", originalIssuer)
		os.Setenv("ASGARDEO_TOKEN_URL", originalTokenURL)
		os.Setenv("PDP_SERVICEURL", originalPDPURLStd)
		os.Setenv("CHOREO_PDP_CONNECTION_CHOREOAPIKEY", originalPDPKey)
	}()

	// Unset env vars
	os.Unsetenv("ASGARDEO_BASE_URL")
	os.Unsetenv("ASGARDEO_CLIENT_ID")
	os.Unsetenv("ASGARDEO_CLIENT_SECRET")
	os.Unsetenv("ASGARDEO_JWKS_URL")
	os.Unsetenv("ASGARDEO_ISSUER")
	os.Unsetenv("ASGARDEO_TOKEN_URL")
	os.Unsetenv("PDP_SERVICEURL")
	os.Unsetenv("CHOREO_PDP_CONNECTION_CHOREOAPIKEY")

	// Test missing IDP config (NewIdpAPIProvider fails)

	// We need a DB connection
	db := services.SetupSQLiteTestDB(t)

	// Case 1: Missing IDP config (BaseURL)
	handler, err := NewV1Handler(db)
	assert.Error(t, err)
	assert.Nil(t, handler)
	assert.Contains(t, err.Error(), "failed to create IDP provider")

	// Set IDP config
	os.Setenv("ASGARDEO_BASE_URL", "https://example.com")
	os.Setenv("ASGARDEO_CLIENT_ID", "client-id")
	os.Setenv("ASGARDEO_CLIENT_SECRET", "client-secret")

	// Case 2: Missing PDP URL
	handler, err = NewV1Handler(db)
	assert.Error(t, err)
	assert.Nil(t, handler)
	assert.Contains(t, err.Error(), "PDP_SERVICEURL environment variable not set")

	// Set PDP URL (standard)
	os.Setenv("PDP_SERVICEURL", "http://pdp:8080")

	// Case 3: Missing PDP Key
	handler, err = NewV1Handler(db)
	assert.Error(t, err)
	assert.Nil(t, handler)
	assert.Contains(t, err.Error(), "CHOREO_PDP_CONNECTION_CHOREOAPIKEY environment variable not set")

	// Set PDP Key
	os.Setenv("CHOREO_PDP_CONNECTION_CHOREOAPIKEY", "api-key")

	// Case 4: Success
	handler, err = NewV1Handler(db)
	assert.NoError(t, err)
	assert.NotNil(t, handler)
}

func TestGetUserMemberID_Caching(t *testing.T) {
	testHandler := NewTestV1Handler(t)

	// Setup mock IDP
	mockIDPStore = new(MockIdentityProviderAPI)
	// Re-create member service with this mock
	testHandler.handler.memberService = services.NewMemberService(testHandler.db, mockIDPStore)

	// Create a user
	user := &models.AuthenticatedUser{
		IdpUserID: "test-user-id",
		Email:     "test@example.com",
	}

	// Mock request
	req := httptest.NewRequest("GET", "/", nil)

	// Case 1: Member not found in DB
	// Note: GetAllMembers uses DB, not IDP, so we don't need to mock IDP for this call

	id, err := testHandler.handler.getUserMemberID(req, user)
	assert.Error(t, err)
	assert.Empty(t, id)
	// MemberService.getFilteredMembers returns error when record not found
	assert.Contains(t, err.Error(), "failed to fetch member")

	// Verify error is cached
	errCached := user.GetCachedMemberIDError()
	assert.Error(t, errCached)
	assert.Equal(t, err, errCached)

	// Case 2: Cached error is returned
	id, err = testHandler.handler.getUserMemberID(req, user)
	assert.Error(t, err)
	assert.Equal(t, errCached, err)

	// Case 3: Member exists
	// Clear cache
	user = &models.AuthenticatedUser{
		IdpUserID: "test-user-id-2",
		Email:     "test2@example.com",
	}

	// Create member in DB
	memberID := createTestMember(t, testHandler.db, "test2@example.com")
	// Update member with correct IdpUserID
	err = testHandler.db.Model(&models.Member{}).Where("member_id = ?", memberID).Update("idp_user_id", "test-user-id-2").Error
	assert.NoError(t, err)

	id, err = testHandler.handler.getUserMemberID(req, user)
	assert.NoError(t, err)
	assert.Equal(t, memberID, id)

	// Verify ID is cached
	cachedID, cached := user.GetCachedMemberID()
	assert.True(t, cached)
	assert.Equal(t, memberID, cachedID)

	// Case 4: Cached ID is returned
	id, err = testHandler.handler.getUserMemberID(req, user)
	assert.NoError(t, err)
	assert.Equal(t, memberID, id)
}

func TestNewV1Handler_StandardOIDC_WithoutBaseURL(t *testing.T) {
	// Save current env vars
	originalBaseURL := os.Getenv("ASGARDEO_BASE_URL")
	originalClientID := os.Getenv("ASGARDEO_CLIENT_ID")
	originalClientSecret := os.Getenv("ASGARDEO_CLIENT_SECRET")
	originalJWKS := os.Getenv("ASGARDEO_JWKS_URL")
	originalIssuer := os.Getenv("ASGARDEO_ISSUER")
	originalTokenURL := os.Getenv("ASGARDEO_TOKEN_URL")
	originalPDPURLStd := os.Getenv("PDP_SERVICEURL")
	originalPDPKey := os.Getenv("CHOREO_PDP_CONNECTION_CHOREOAPIKEY")

	// Restore env vars after test
	defer func() {
		os.Setenv("ASGARDEO_BASE_URL", originalBaseURL)
		os.Setenv("ASGARDEO_CLIENT_ID", originalClientID)
		os.Setenv("ASGARDEO_CLIENT_SECRET", originalClientSecret)
		os.Setenv("ASGARDEO_JWKS_URL", originalJWKS)
		os.Setenv("ASGARDEO_ISSUER", originalIssuer)
		os.Setenv("ASGARDEO_TOKEN_URL", originalTokenURL)
		os.Setenv("PDP_SERVICEURL", originalPDPURLStd)
		os.Setenv("CHOREO_PDP_CONNECTION_CHOREOAPIKEY", originalPDPKey)
	}()

	// Unset ASGARDEO_BASE_URL
	os.Unsetenv("ASGARDEO_BASE_URL")

	// Configure standard OIDC
	os.Setenv("ASGARDEO_JWKS_URL", "https://example.com/oauth2/jwks")
	os.Setenv("ASGARDEO_ISSUER", "https://example.com")
	os.Setenv("ASGARDEO_CLIENT_ID", "client-id")
	os.Setenv("ASGARDEO_CLIENT_SECRET", "client-secret")
	os.Setenv("PDP_SERVICEURL", "http://pdp:8080")
	os.Setenv("CHOREO_PDP_CONNECTION_CHOREOAPIKEY", "api-key")

	db := services.SetupSQLiteTestDB(t)

	handler, err := NewV1Handler(db)
	assert.NoError(t, err)
	assert.NotNil(t, handler)
}
