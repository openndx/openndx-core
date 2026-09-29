package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Token represents a cached OAuth2 token set, along with the token endpoint
// and client ID needed to refresh it later without the caller re-specifying flags.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenURL     string    `json:"token_url"`
	ClientID     string    `json:"client_id"`
	// ExtraParams remembers IDP-specific query params required at login (e.g.
	// ThunderID's resource=... audience binding) so a later refresh reuses
	// them automatically instead of silently dropping them.
	ExtraParams map[string]string `json:"extra_params,omitempty"`
}

// Expired reports whether the access token has passed its expiry, with a
// small safety margin so a request doesn't race the real expiry.
func (t *Token) Expired() bool {
	return time.Now().Add(30 * time.Second).After(t.ExpiresAt)
}

// DefaultCredentialsPath returns the default location for cached CLI
// credentials for the given profile name. Each non-default profile gets its
// own file - they're logins against different identity providers/clients,
// so caching them together would let switching profiles silently pick up
// the wrong token. The default profile keeps the original, un-suffixed path
// for backward compatibility with credentials cached before profiles existed.
func DefaultCredentialsPath(profileName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}
	if profileName == "" || profileName == "local" {
		return filepath.Join(home, ".openndx", "credentials.json"), nil
	}
	// profileName can come from a config file, --profile flag, or NDX_PROFILE
	// env var - reject path separators so it can't be used to write the
	// cached token outside the .openndx directory (e.g. "../../.ssh/foo").
	if strings.ContainsAny(profileName, "/\\") {
		return "", fmt.Errorf("invalid profile name %q: must not contain path separators", profileName)
	}
	return filepath.Join(home, ".openndx", fmt.Sprintf("credentials-%s.json", profileName)), nil
}

// SaveToken writes the token to the given path, creating parent directories
// as needed. The file is written with 0600 permissions since it holds secrets.
func SaveToken(path string, token *Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create credentials directory: %w", err)
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}
	return nil
}

// LoadToken reads a cached token from the given path.
func LoadToken(path string) (*Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file: %w", err)
	}
	return &token, nil
}
