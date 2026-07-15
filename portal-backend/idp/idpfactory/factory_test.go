package idpfactory

import (
	"context"
	"testing"

	"github.com/OpenNDX/openndx-core/portal-backend/idp"
	"github.com/stretchr/testify/assert"
)

func TestNewIdpAPIProvider(t *testing.T) {
	t.Run("AsgardeoProvider", func(t *testing.T) {
		cfg := FactoryConfig{
			ProviderType: idp.ProviderAsgardeo,
			BaseURL:      "https://api.asgardeo.io/t/testorg",
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			Scopes:       []string{"scope1", "scope2"},
		}

		provider, err := NewIdpAPIProvider(cfg)

		assert.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("UnsupportedProvider", func(t *testing.T) {
		cfg := FactoryConfig{
			ProviderType: idp.ProviderType("unsupported"),
			BaseURL:      "https://api.example.com",
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		}

		provider, err := NewIdpAPIProvider(cfg)

		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "unsupported provider type")
	})
}

func TestAsgardeoAdapter_AddMemberToGroup(t *testing.T) {
	// Test that the adapter correctly implements the interface
	// The adapter converts string groupID to *string for the underlying client
	t.Run("AdapterImplementsInterface", func(t *testing.T) {
		cfg := FactoryConfig{
			ProviderType: idp.ProviderAsgardeo,
			BaseURL:      "https://api.asgardeo.io/t/testorg",
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			Scopes:       []string{},
		}

		provider, err := NewIdpAPIProvider(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, provider)

		// Verify the provider implements IdentityProviderAPI which embeds GroupManager
		// This tests that AddMemberToGroup signature is correct
		ctx := context.Background()
		groupID := "group-123"
		member := &idp.GroupMember{
			Value:   "user-123",
			Display: "Test User",
		}

		// IdentityProviderAPI embeds GroupManager, so AddMemberToGroup should be available
		// This will fail at runtime without real Asgardeo, but tests the adapter signature
		groupManager, ok := provider.(idp.GroupManager)
		assert.True(t, ok, "Provider should implement GroupManager")
		if ok {
			err = groupManager.AddMemberToGroup(ctx, groupID, member)
			// We expect an error since we don't have a real Asgardeo instance
			assert.Error(t, err)
		}
	})
}
