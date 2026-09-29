// Package profile manages named, reusable sets of ndx CLI flag defaults
// (identity provider issuer, client ID, scopes, callback port, Portal
// Backend URL, TLS verification) cached at ~/.openndx/config.json. Profiles let
// an operator switch between environments (local dev, staging, a partner's
// deployment) without retyping every flag on every command; any flag passed
// explicitly on the command line still overrides the active profile.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultName is the profile ondx falls back to when no config file exists
// yet and none is named explicitly, pre-populated with this repo's
// docker-compose local-dev stack (ThunderID as IDP, Portal Backend on
// :8083) so `ondx login` works with zero flags out of the box.
const DefaultName = "local"

// ThunderIDCLIClientID and ThunderIDCallbackPort are the ndx CLI's OAuth2
// client ID and redirect callback port as registered with this repo's local
// ThunderID instance (thunderid/bootstrap/application.yaml). ThunderID
// rejects any redirect URI other than the exact one registered there
// (http://127.0.0.1:8765/callback) - a wildcard/wrong port fails login - so
// `ondx login` must reject any other --callback-port when logging in as this
// client, rather than silently trying (and failing) against ThunderID.
const (
	ThunderIDCLIClientID  = "NDX_CLI"
	ThunderIDCallbackPort = 8765
)

var defaultLocalProfile = Profile{
	Issuer:       "http://localhost:8090",
	ClientID:     ThunderIDCLIClientID,
	Scopes:       "openid roles email",
	CallbackPort: ThunderIDCallbackPort,
	PBURL:        "http://localhost:8083",
	Insecure:     true,
}

// Profile is one named set of flag defaults. Every field is optional -
// callers use it purely to seed a flag's default value, so a zero value
// simply falls through to whatever the caller does when a flag is unset.
type Profile struct {
	Issuer       string `json:"issuer,omitempty"`
	AuthURL      string `json:"auth_url,omitempty"`
	TokenURL     string `json:"token_url,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	Scopes       string `json:"scopes,omitempty"`
	CallbackPort int    `json:"callback_port,omitempty"`
	PBURL        string `json:"pb_url,omitempty"`
	Insecure     bool   `json:"insecure,omitempty"`
}

// Config is the on-disk shape of ~/.openndx/config.json.
type Config struct {
	CurrentProfile string             `json:"current_profile"`
	Profiles       map[string]Profile `json:"profiles"`
}

// DefaultConfigPath returns the default location for the profile config file.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}
	return filepath.Join(home, ".openndx", "config.json"), nil
}

// Load reads the config file at path. A missing file is not an error - it
// yields a config with just the built-in default profile, so a first-time
// user gets working defaults without ever running `ondx profile set`. The
// default profile is likewise injected whenever the file exists but doesn't
// define one of its own under DefaultName.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{
			CurrentProfile: DefaultName,
			Profiles:       map[string]Profile{DefaultName: defaultLocalProfile},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if _, ok := cfg.Profiles[DefaultName]; !ok {
		cfg.Profiles[DefaultName] = defaultLocalProfile
	}
	if cfg.CurrentProfile == "" {
		cfg.CurrentProfile = DefaultName
	}
	return &cfg, nil
}

// Save writes cfg to path, creating parent directories as needed.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// Get returns the named profile, or an error if it isn't defined.
func (c *Config) Get(name string) (Profile, error) {
	p, ok := c.Profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("no such profile %q (run 'ondx profile list' to see available profiles)", name)
	}
	return p, nil
}
