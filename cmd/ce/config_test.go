// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenNSW/core/cors"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalConfig holds only the settings that have no default.
const minimalConfig = `
consentPortalURL: https://consent.example
idp:
  jwksURL: https://idp.example/oauth2/jwks
  issuer: https://idp.example
  audience: CONSENT_PORTAL
database:
  postgres:
    host: db.internal
    user: ce
    password: "{{env:TEST_CE_DB_PASSWORD}}"
`

// writeConfig writes content to a config file in a temporary directory and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadConfigAppliesDefaults(t *testing.T) {
	t.Setenv("TEST_CE_DB_PASSWORD", "s3cret")

	cfg, err := loadConfig([]string{"-config", writeConfig(t, minimalConfig)})
	require.NoError(t, err)

	assert.Equal(t, 8081, cfg.Server.Port)
	assert.Equal(t, 10*time.Second, cfg.Server.Timeout)
	assert.Empty(t, cfg.Environment)
	assert.Equal(t, logging.Config{Level: "info", Format: "json"}, cfg.Logging)
	assert.Equal(t, "sub", cfg.IDP.SubjectClaim)
	assert.False(t, cfg.IDP.InsecureSkipTLSVerify)
	assert.Equal(t, 30*time.Minute, cfg.Database.IdleTimeout)

	assert.Equal(t, cors.Config{
		AllowedOrigins: []string{"https://consent.example"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowedHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-Requested-With", "X-CSRF-Token", "X-Request-ID",
		},
		AllowCredentials: true,
		MaxAge:           86400,
	}, cfg.CORS, "the consent portal is always an allowed origin")

	pg := cfg.Database.Postgres
	require.NotNil(t, pg)
	assert.Equal(t, "db.internal", pg.Host)
	assert.Equal(t, "ce", pg.User)
	assert.Equal(t, "s3cret", pg.Password, "the password placeholder is read from the environment")
	assert.Equal(t, 5432, pg.Port)
	assert.Equal(t, "consent_engine", pg.Name)
	assert.Equal(t, "require", pg.SSLMode)
	assert.Equal(t, 25, pg.Pool.MaxOpenConns)
	assert.Equal(t, 5, pg.Pool.MaxIdleConns)
	assert.Equal(t, 3600, pg.Pool.MaxConnLifetimeSeconds)
}

func TestLoadConfigReadsEverySetting(t *testing.T) {
	path := writeConfig(t, `
server:
  port: 9081
  timeout: 3s
environment: staging
logging:
  level: warn
  format: text
consentPortalURL: https://consent.example
cors:
  allowedOrigins:
    - https://admin.example
  allowedMethods: [GET, PUT]
  allowedHeaders: [Authorization]
  allowCredentials: false
  maxAge: 600
idp:
  jwksURL: https://idp.example/oauth2/jwks
  issuer: https://idp.example
  audience: CONSENT_PORTAL
  subjectClaim: email
  insecureSkipTLSVerify: true
database:
  driver: postgres
  postgres:
    host: db.internal
    port: 6543
    user: ce
    password: pw
    name: consent
    sslMode: verify-full
    pool:
      maxOpenConns: 50
      maxIdleConns: 10
      maxConnLifetimeSeconds: 600
  idleTimeout: 5m
`)

	cfg, err := loadConfig([]string{"-config", path})
	require.NoError(t, err)

	assert.Equal(t, ServerConfig{Port: 9081, Timeout: 3 * time.Second}, cfg.Server)
	assert.Equal(t, "staging", cfg.Environment)
	assert.Equal(t, logging.Config{Level: "warn", Format: "text"}, cfg.Logging)
	assert.Equal(t, "https://consent.example", cfg.ConsentPortalURL)
	assert.Equal(t, cors.Config{
		AllowedOrigins:   []string{"https://admin.example", "https://consent.example"},
		AllowedMethods:   []string{"GET", "PUT"},
		AllowedHeaders:   []string{"Authorization"},
		AllowCredentials: false,
		MaxAge:           600,
	}, cfg.CORS, "lists in the file replace the defaults")
	assert.Equal(t, IDPConfig{
		Config: authn.Config{
			JWKSURL:               "https://idp.example/oauth2/jwks",
			Issuer:                "https://idp.example",
			Audience:              "CONSENT_PORTAL",
			InsecureSkipTLSVerify: true,
		},
		SubjectClaim: "email",
	}, cfg.IDP)
	assert.Equal(t, 5*time.Minute, cfg.Database.IdleTimeout)

	pg := cfg.Database.Postgres
	assert.Equal(t, 6543, pg.Port)
	assert.Equal(t, "consent", pg.Name)
	assert.Equal(t, "verify-full", pg.SSLMode)
	assert.Equal(t, 50, pg.Pool.MaxOpenConns)
	assert.Equal(t, 10, pg.Pool.MaxIdleConns)
	assert.Equal(t, 600, pg.Pool.MaxConnLifetimeSeconds)
}

func TestLoadConfigDoesNotDuplicateConsentPortalOrigin(t *testing.T) {
	t.Setenv("TEST_CE_DB_PASSWORD", "s3cret")
	path := writeConfig(t, minimalConfig+"cors:\n  allowedOrigins: [https://consent.example]\n")

	cfg, err := loadConfig([]string{"-config", path})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://consent.example"}, cfg.CORS.AllowedOrigins)
}

func TestLoadConfigAllowsConsentPortalOriginOnly(t *testing.T) {
	t.Setenv("TEST_CE_DB_PASSWORD", "s3cret")
	path := writeConfig(t, strings.Replace(minimalConfig,
		"consentPortalURL: https://consent.example",
		"consentPortalURL: https://consent.example:8443/portal?lang=en", 1))

	cfg, err := loadConfig([]string{"-config", path})
	require.NoError(t, err)
	assert.Equal(t, "https://consent.example:8443/portal?lang=en", cfg.ConsentPortalURL)
	assert.Equal(t, []string{"https://consent.example:8443"}, cfg.CORS.AllowedOrigins)
}

func TestLoadConfigPathFromEnvironment(t *testing.T) {
	t.Setenv("TEST_CE_DB_PASSWORD", "s3cret")
	t.Setenv("CONFIG_PATH", writeConfig(t, minimalConfig))

	cfg, err := loadConfig(nil)
	require.NoError(t, err)
	assert.Equal(t, "db.internal", cfg.Database.Postgres.Host)
}

func TestLoadConfigFlagOverridesEnvironment(t *testing.T) {
	t.Setenv("TEST_CE_DB_PASSWORD", "s3cret")
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
	// replace swaps one line of minimalConfig, so each case breaks one setting.
	replace := func(line, with string) string {
		return strings.Replace(minimalConfig, line, with, 1)
	}
	pw := map[string]string{"TEST_CE_DB_PASSWORD": "pw"}

	tests := []struct {
		name    string
		content string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "unset password variable",
			content: minimalConfig,
			wantErr: "TEST_CE_DB_PASSWORD",
		},
		{
			name:    "malformed YAML",
			content: "server: [",
			wantErr: "parsing config file",
		},
		{
			name:    "server port out of range",
			content: minimalConfig + "server:\n  port: 70000\n",
			env:     pw,
			wantErr: "server.port",
		},
		{
			name:    "non-positive timeout",
			content: minimalConfig + "server:\n  timeout: 0s\n",
			env:     pw,
			wantErr: "server.timeout",
		},
		{
			name:    "invalid duration",
			content: minimalConfig + "server:\n  timeout: soon\n",
			env:     pw,
			wantErr: "parsing config file",
		},
		{
			name:    "unknown log level",
			content: minimalConfig + "logging:\n  level: verbose\n",
			env:     pw,
			wantErr: "logging.level",
		},
		{
			name:    "unknown log format",
			content: minimalConfig + "logging:\n  format: xml\n",
			env:     pw,
			wantErr: "logging.format",
		},
		{
			name:    "missing consent portal URL",
			content: replace("consentPortalURL: https://consent.example", ""),
			env:     pw,
			wantErr: "consentPortalURL",
		},
		{
			name:    "consent portal URL without scheme",
			content: replace("consentPortalURL: https://consent.example", "consentPortalURL: consent.example"),
			env:     pw,
			wantErr: "consentPortalURL",
		},
		{
			name:    "wildcard origin with credentials",
			content: minimalConfig + "cors:\n  allowedOrigins: [\"*\"]\n",
			env:     pw,
			wantErr: "invalid cors configuration",
		},
		{
			name:    "origin that is not a URL",
			content: minimalConfig + "cors:\n  allowedOrigins: [admin]\n",
			env:     pw,
			wantErr: "invalid cors configuration",
		},
		{
			name:    "missing JWKS URL",
			content: replace("  jwksURL: https://idp.example/oauth2/jwks\n", ""),
			env:     pw,
			wantErr: "jwksURL is required",
		},
		{
			name:    "missing issuer",
			content: replace("  issuer: https://idp.example\n", ""),
			env:     pw,
			wantErr: "issuer is required",
		},
		{
			name:    "missing audience",
			content: replace("  audience: CONSENT_PORTAL\n", ""),
			env:     pw,
			wantErr: "audience is required",
		},
		{
			name:    "empty subject claim",
			content: replace("  audience: CONSENT_PORTAL\n", "  audience: CONSENT_PORTAL\n  subjectClaim: \"\"\n"),
			env:     pw,
			wantErr: "idp.subjectClaim",
		},
		{
			name:    "missing database host",
			content: replace("    host: db.internal\n", ""),
			env:     pw,
			wantErr: "postgres host is required",
		},
		{
			name:    "missing database password",
			content: replace("    password: \"{{env:TEST_CE_DB_PASSWORD}}\"\n", ""),
			wantErr: "postgres password is required",
		},
		{
			name:    "unsupported driver",
			content: replace("database:\n", "database:\n  driver: sqlite\n  sqlite:\n    path: ce.db\n"),
			env:     pw,
			wantErr: "database.driver must be postgres",
		},
		{
			name:    "database port out of range",
			content: minimalConfig + "    port: 0\n",
			env:     pw,
			wantErr: "database.postgres.port",
		},
		{
			name:    "empty SSL mode",
			content: minimalConfig + "    sslMode: \"\"\n",
			env:     pw,
			wantErr: "database.postgres.sslMode",
		},
		{
			name:    "more idle than open connections",
			content: minimalConfig + "    pool:\n      maxOpenConns: 2\n      maxIdleConns: 3\n",
			env:     pw,
			wantErr: "database.postgres.pool",
		},
		{
			name:    "non-positive idle timeout",
			content: minimalConfig + "  idleTimeout: 0s\n",
			env:     pw,
			wantErr: "database.idleTimeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_CE_DB_PASSWORD", "")
			require.NoError(t, os.Unsetenv("TEST_CE_DB_PASSWORD"))
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
	t.Setenv("DB_NAME_CE", "consent_engine")
	t.Setenv("CONSENT_PORTAL_URL", "http://localhost:5173")
	t.Setenv("IDP_PUBLIC_URL", "http://localhost:8090")
	t.Setenv("CONSENT_ENGINE_AUDIENCE", "CONSENT_PORTAL")
	t.Setenv("IDP_SUBJECT_CLAIM", "email")

	for _, path := range []string{"config.example.yaml", "config.docker.yaml", "../../tests/integration/ce-config.yaml"} {
		t.Run(path, func(t *testing.T) {
			cfg, err := loadConfig([]string{"-config", path})
			require.NoError(t, err)
			assert.Contains(t, cfg.CORS.AllowedOrigins, cfg.ConsentPortalURL)
		})
	}
}
