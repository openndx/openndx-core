package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// generateRandomURLSafeString returns a cryptographically random, base64url
// (no padding) encoded string derived from n random bytes.
func generateRandomURLSafeString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// PKCE holds an OAuth2 PKCE (RFC 7636) verifier/challenge pair using the
// S256 challenge method.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a new PKCE verifier/challenge pair.
func NewPKCE() (*PKCE, error) {
	verifier, err := generateRandomURLSafeString(64)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return &PKCE{Verifier: verifier, Challenge: challenge}, nil
}

// NewState generates a random CSRF state parameter for the authorization request.
func NewState() (string, error) {
	return generateRandomURLSafeString(24)
}
