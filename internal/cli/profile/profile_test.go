package profile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_MissingFileYieldsBuiltinDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	assert.NoError(t, err)
	assert.Equal(t, DefaultName, cfg.CurrentProfile)
	local, err := cfg.Get(DefaultName)
	assert.NoError(t, err)
	assert.Equal(t, "http://localhost:8090", local.Issuer)
	assert.Equal(t, "NDX_CLI", local.ClientID)
	assert.Equal(t, 8765, local.CallbackPort)
	assert.True(t, local.Insecure)
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")

	cfg := &Config{
		CurrentProfile: "staging",
		Profiles: map[string]Profile{
			"staging": {
				Issuer:   "https://idp.staging.example.com",
				ClientID: "ndx-staging",
				PBURL:    "https://pb.staging.example.com",
			},
		},
	}
	assert.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	assert.NoError(t, err)
	assert.Equal(t, "staging", loaded.CurrentProfile)

	staging, err := loaded.Get("staging")
	assert.NoError(t, err)
	assert.Equal(t, "https://idp.staging.example.com", staging.Issuer)
	assert.Equal(t, "ndx-staging", staging.ClientID)

	// The built-in default profile is still injected even though the file
	// only ever defined "staging".
	local, err := loaded.Get(DefaultName)
	assert.NoError(t, err)
	assert.Equal(t, "http://localhost:8090", local.Issuer)
}

func TestLoad_PreservesUserOverriddenLocalProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{
		CurrentProfile: DefaultName,
		Profiles: map[string]Profile{
			DefaultName: {Issuer: "https://custom.local.example.com"},
		},
	}
	assert.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	assert.NoError(t, err)
	local, err := loaded.Get(DefaultName)
	assert.NoError(t, err)
	// Must not be clobbered back to the built-in default.
	assert.Equal(t, "https://custom.local.example.com", local.Issuer)
}

func TestGet_UnknownProfile(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{}}
	_, err := cfg.Get("nope")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "no such profile")
}
