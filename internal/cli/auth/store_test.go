package auth

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSaveAndLoadToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "credentials.json")

	token := &Token{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		TokenURL:     "https://idp.example.com/oauth2/token",
		ClientID:     "client-abc",
	}

	err := SaveToken(path, token)
	assert.NoError(t, err)

	loaded, err := LoadToken(path)
	assert.NoError(t, err)
	assert.Equal(t, token.AccessToken, loaded.AccessToken)
	assert.Equal(t, token.RefreshToken, loaded.RefreshToken)
	assert.Equal(t, token.TokenType, loaded.TokenType)
	assert.True(t, token.ExpiresAt.Equal(loaded.ExpiresAt))
	assert.Equal(t, token.TokenURL, loaded.TokenURL)
	assert.Equal(t, token.ClientID, loaded.ClientID)
}

func TestLoadToken_MissingFile(t *testing.T) {
	_, err := LoadToken(filepath.Join(t.TempDir(), "does-not-exist.json"))
	assert.Error(t, err)
}

func TestToken_Expired(t *testing.T) {
	expired := &Token{ExpiresAt: time.Now().Add(-time.Minute)}
	assert.True(t, expired.Expired())

	valid := &Token{ExpiresAt: time.Now().Add(time.Hour)}
	assert.False(t, valid.Expired())

	// Within the 30s safety margin should count as expired.
	almostExpired := &Token{ExpiresAt: time.Now().Add(10 * time.Second)}
	assert.True(t, almostExpired.Expired())
}

func TestDefaultCredentialsPath(t *testing.T) {
	t.Run("default profile keeps the original unsuffixed path", func(t *testing.T) {
		path, err := DefaultCredentialsPath("local")
		assert.NoError(t, err)
		assert.Contains(t, path, ".openndx")
		assert.True(t, strings.HasSuffix(path, "credentials.json"))
	})

	t.Run("empty profile name also falls back to the unsuffixed path", func(t *testing.T) {
		path, err := DefaultCredentialsPath("")
		assert.NoError(t, err)
		assert.True(t, strings.HasSuffix(path, "credentials.json"))
	})

	t.Run("named profile gets its own file", func(t *testing.T) {
		path, err := DefaultCredentialsPath("staging")
		assert.NoError(t, err)
		assert.Contains(t, path, ".openndx")
		assert.True(t, strings.HasSuffix(path, "credentials-staging.json"))
	})

	t.Run("rejects profile names containing a path separator", func(t *testing.T) {
		_, err := DefaultCredentialsPath("../../.ssh/id_rsa")
		assert.Error(t, err)
		assert.ErrorContains(t, err, "must not contain path separators")

		_, err = DefaultCredentialsPath(`staging\..\..\secrets`)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "must not contain path separators")
	})
}
