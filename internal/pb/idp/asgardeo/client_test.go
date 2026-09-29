package asgardeo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	t.Run("WithScopes", func(t *testing.T) {
		client := NewClient("https://api.asgardeo.io/t/testorg", "client-id", "client-secret", []string{"scope1", "scope2"})
		assert.NotNil(t, client)
		assert.Equal(t, "https://api.asgardeo.io/t/testorg", client.BaseURL)
		assert.NotNil(t, client.OAuthConfig)
		assert.Equal(t, "client-id", client.OAuthConfig.ClientID)
		assert.Equal(t, "client-secret", client.OAuthConfig.ClientSecret)
		assert.Equal(t, []string{"scope1", "scope2"}, client.OAuthConfig.Scopes)
		assert.Equal(t, "https://api.asgardeo.io/t/testorg/oauth2/token", client.OAuthConfig.TokenURL)
		assert.NotNil(t, client.Client)
	})

	t.Run("WithoutScopes", func(t *testing.T) {
		client := NewClient("https://api.asgardeo.io/t/testorg", "client-id", "client-secret", []string{})
		assert.NotNil(t, client)
		assert.Equal(t, []string{}, client.OAuthConfig.Scopes)
	})

	t.Run("WithEmptyBaseURL", func(t *testing.T) {
		client := NewClient("", "client-id", "client-secret", []string{})
		assert.NotNil(t, client)
		assert.Equal(t, "/oauth2/token", client.OAuthConfig.TokenURL)
	})
}
