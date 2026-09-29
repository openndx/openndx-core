package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPKCE(t *testing.T) {
	pkce, err := NewPKCE()
	assert.NoError(t, err)
	assert.NotEmpty(t, pkce.Verifier)
	assert.NotEmpty(t, pkce.Challenge)

	// RFC 7636 requires the verifier to be 43-128 characters.
	assert.GreaterOrEqual(t, len(pkce.Verifier), 43)
	assert.LessOrEqual(t, len(pkce.Verifier), 128)

	// Challenge must be the base64url(SHA256(verifier)) of the verifier.
	sum := sha256.Sum256([]byte(pkce.Verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	assert.Equal(t, expected, pkce.Challenge)
}

func TestNewPKCE_Unique(t *testing.T) {
	a, err := NewPKCE()
	assert.NoError(t, err)
	b, err := NewPKCE()
	assert.NoError(t, err)

	assert.NotEqual(t, a.Verifier, b.Verifier)
	assert.NotEqual(t, a.Challenge, b.Challenge)
}

func TestNewState(t *testing.T) {
	a, err := NewState()
	assert.NoError(t, err)
	assert.NotEmpty(t, a)

	b, err := NewState()
	assert.NoError(t, err)
	assert.NotEqual(t, a, b)
}
