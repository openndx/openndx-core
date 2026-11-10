package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openndx/openndx-core/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalConfig holds only the settings that have no default.
const minimalConfig = `
database:
  postgres:
    host: db.internal
    user: pdp
    password: "{{env:TEST_PDP_DB_PASSWORD}}"
`

// writeConfig writes content to a config file in a temporary directory and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadConfigAppliesDefaults(t *testing.T) {
	t.Setenv("TEST_PDP_DB_PASSWORD", "s3cret")

	cfg, err := loadConfig([]string{"-config", writeConfig(t, minimalConfig)})
	require.NoError(t, err)

	assert.Equal(t, 8082, cfg.Server.Port)
	assert.Equal(t, 10*time.Second, cfg.Server.Timeout)
	assert.Empty(t, cfg.Environment)
	assert.Equal(t, logging.Config{Level: "info", Format: "json"}, cfg.Logging)
	assert.Equal(t, 30*time.Minute, cfg.Database.IdleTimeout)
	assert.Empty(t, cfg.Policy.ModulesDir, "only the built-in policy classes are loaded by default")

	pg := cfg.Database.Postgres
	require.NotNil(t, pg)
	assert.Equal(t, "db.internal", pg.Host)
	assert.Equal(t, "pdp", pg.User)
	assert.Equal(t, "s3cret", pg.Password, "the password placeholder is read from the environment")
	assert.Equal(t, 5432, pg.Port)
	assert.Equal(t, "pdp", pg.Name)
	assert.Equal(t, "require", pg.SSLMode)
	assert.Equal(t, 25, pg.Pool.MaxOpenConns)
	assert.Equal(t, 5, pg.Pool.MaxIdleConns)
	assert.Equal(t, 3600, pg.Pool.MaxConnLifetimeSeconds)
}

func TestLoadConfigReadsEverySetting(t *testing.T) {
	path := writeConfig(t, `
server:
  port: 9090
  timeout: 3s
environment: staging
logging:
  level: warn
  format: text
database:
  driver: postgres
  postgres:
    host: db.internal
    port: 6543
    user: pdp
    password: pw
    name: policy
    sslMode: verify-full
    pool:
      maxOpenConns: 50
      maxIdleConns: 10
      maxConnLifetimeSeconds: 600
  idleTimeout: 5m
policy:
  modulesDir: /etc/pdp/policies
`)

	cfg, err := loadConfig([]string{"-config", path})
	require.NoError(t, err)

	assert.Equal(t, "/etc/pdp/policies", cfg.Policy.ModulesDir)
	assert.Equal(t, ServerConfig{Port: 9090, Timeout: 3 * time.Second}, cfg.Server)
	assert.Equal(t, "staging", cfg.Environment)
	assert.Equal(t, logging.Config{Level: "warn", Format: "text"}, cfg.Logging)
	assert.Equal(t, 5*time.Minute, cfg.Database.IdleTimeout)

	pg := cfg.Database.Postgres
	assert.Equal(t, 6543, pg.Port)
	assert.Equal(t, "policy", pg.Name)
	assert.Equal(t, "verify-full", pg.SSLMode)
	assert.Equal(t, 50, pg.Pool.MaxOpenConns)
	assert.Equal(t, 10, pg.Pool.MaxIdleConns)
	assert.Equal(t, 600, pg.Pool.MaxConnLifetimeSeconds)
}

func TestLoadConfigPathFromEnvironment(t *testing.T) {
	t.Setenv("TEST_PDP_DB_PASSWORD", "s3cret")
	t.Setenv("CONFIG_PATH", writeConfig(t, minimalConfig))

	cfg, err := loadConfig(nil)
	require.NoError(t, err)
	assert.Equal(t, "db.internal", cfg.Database.Postgres.Host)
}

func TestLoadConfigFlagOverridesEnvironment(t *testing.T) {
	t.Setenv("TEST_PDP_DB_PASSWORD", "s3cret")
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))

	cfg, err := loadConfig([]string{"-config", writeConfig(t, minimalConfig)})
	require.NoError(t, err)
	assert.Equal(t, "db.internal", cfg.Database.Postgres.Host)
}

func TestLoadConfigDefaultPath(t *testing.T) {
	t.Setenv("CONFIG_PATH", "")
	assert.Equal(t, "config.yaml", configPathFromEnv())
}

func TestLoadConfigHelp(t *testing.T) {
	_, err := loadConfig([]string{"-h"})
	assert.True(t, errors.Is(err, flag.ErrHelp))
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "unset password variable",
			content: minimalConfig,
			wantErr: "TEST_PDP_DB_PASSWORD",
		},
		{
			name:    "malformed YAML",
			content: "server: [",
			wantErr: "parsing config file",
		},
		{
			name:    "missing database host",
			content: "database:\n  postgres:\n    user: pdp\n    password: pw\n",
			wantErr: "postgres host is required",
		},
		{
			name:    "missing database password",
			content: "database:\n  postgres:\n    host: db\n    user: pdp\n",
			wantErr: "postgres password is required",
		},
		{
			name:    "unsupported driver",
			content: "database:\n  driver: sqlite\n  sqlite:\n    path: pdp.db\n",
			wantErr: "database.driver must be postgres",
		},
		{
			name:    "server port out of range",
			content: minimalConfig + "server:\n  port: 70000\n",
			env:     map[string]string{"TEST_PDP_DB_PASSWORD": "pw"},
			wantErr: "server.port",
		},
		{
			name:    "non-positive timeout",
			content: minimalConfig + "server:\n  timeout: 0s\n",
			env:     map[string]string{"TEST_PDP_DB_PASSWORD": "pw"},
			wantErr: "server.timeout",
		},
		{
			name:    "invalid duration",
			content: minimalConfig + "server:\n  timeout: soon\n",
			env:     map[string]string{"TEST_PDP_DB_PASSWORD": "pw"},
			wantErr: "parsing config file",
		},
		{
			name:    "unknown log level",
			content: minimalConfig + "logging:\n  level: verbose\n",
			env:     map[string]string{"TEST_PDP_DB_PASSWORD": "pw"},
			wantErr: "logging.level",
		},
		{
			name:    "unknown log format",
			content: minimalConfig + "logging:\n  format: xml\n",
			env:     map[string]string{"TEST_PDP_DB_PASSWORD": "pw"},
			wantErr: "logging.format",
		},
		{
			name:    "database port out of range",
			content: "database:\n  postgres:\n    host: db\n    user: pdp\n    password: pw\n    port: 0\n",
			wantErr: "database.postgres.port",
		},
		{
			name:    "empty SSL mode",
			content: "database:\n  postgres:\n    host: db\n    user: pdp\n    password: pw\n    sslMode: \"\"\n",
			wantErr: "database.postgres.sslMode",
		},
		{
			name:    "more idle than open connections",
			content: "database:\n  postgres:\n    host: db\n    user: pdp\n    password: pw\n    pool:\n      maxOpenConns: 2\n      maxIdleConns: 3\n",
			wantErr: "database.postgres.pool",
		},
		{
			name:    "non-positive idle timeout",
			content: "database:\n  postgres:\n    host: db\n    user: pdp\n    password: pw\n  idleTimeout: 0s\n",
			wantErr: "database.idleTimeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_PDP_DB_PASSWORD", "")
			require.NoError(t, os.Unsetenv("TEST_PDP_DB_PASSWORD"))
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err := loadConfig([]string{"-config", writeConfig(t, tt.content)})
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tt.wantErr), "error %q should mention %q", err, tt.wantErr)
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := loadConfig([]string{"-config", filepath.Join(t.TempDir(), "missing.yaml")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestCommittedConfigsLoad(t *testing.T) {
	t.Setenv("DB_USER", "postgres")
	t.Setenv("DB_PASSWORD", "pw")

	for _, path := range []string{"config.example.yaml", "config.docker.yaml", "../../tests/integration/pdp-config.yaml"} {
		t.Run(path, func(t *testing.T) {
			_, err := loadConfig([]string{"-config", path})
			require.NoError(t, err)
		})
	}
}
