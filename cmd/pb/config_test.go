// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/telemetry"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/logging"
	"github.com/openndx/openndx-core/internal/pb/idp/thunderid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalConfig holds only the settings that have no default.
const minimalConfig = `
idp:
  jwksURL: https://idp.example/oauth2/jwks
  issuer: https://idp.example
  audience: http://pb.example
  management:
    baseURL: https://idp.example
    clientID: PORTAL_BACKEND
    clientSecret: client-secret
pdp:
  url: http://pdp.internal:8082
database:
  postgres:
    host: db.internal
    user: pb
    password: "{{env:TEST_PB_DB_PASSWORD}}"
`

// writeConfig writes content to a config file in a temporary directory and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// withOtelExample returns the path of a config like the one at path with its
// otel example turned on, so the documented block is checked as well. The
// committed configs keep the example commented out; when it is already
// uncommented, as CONTRIBUTING.md#viewing-audit-records has developers do
// locally, the file is used as it is.
func withOtelExample(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")
	start := slices.IndexFunc(lines, func(line string) bool {
		return strings.TrimSuffix(line, "\r") == "# otel:"
	})
	if start == -1 {
		return path
	}
	for i := start; i < len(lines) && strings.HasPrefix(lines[i], "# "); i++ {
		lines[i] = strings.TrimPrefix(lines[i], "# ")
	}
	return writeConfig(t, strings.Join(lines, "\n"))
}

func TestLoadConfigAppliesDefaults(t *testing.T) {
	t.Setenv("TEST_PB_DB_PASSWORD", "p@ss word'")

	cfg, err := loadConfig([]string{"-config", writeConfig(t, minimalConfig)})
	require.NoError(t, err)

	assert.Equal(t, ServerConfig{Port: 8083, Timeout: 15 * time.Second}, cfg.Server)
	assert.Empty(t, cfg.Environment)
	assert.Equal(t, logging.Config{Level: "info", Format: "json"}, cfg.Logging)
	assert.Equal(t, cors.Config{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowedHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-Requested-With", "X-CSRF-Token", "X-Request-ID",
		},
		AllowCredentials: true,
		MaxAge:           86400,
	}, cfg.CORS)
	assert.False(t, cfg.IDP.InsecureSkipTLSVerify)
	assert.Empty(t, cfg.IDP.Management.Scopes)
	assert.Equal(t, telemetry.Config{ServiceName: "portal-backend"}, cfg.Otel, "export, and so audit, is off by default")
	assert.Equal(t, 30*time.Minute, cfg.Database.IdleTimeout)

	pg := cfg.Database.Postgres
	require.NotNil(t, pg)
	assert.Equal(t, "db.internal", pg.Host)
	assert.Equal(t, "pb", pg.User)
	assert.Equal(t, "p@ss word'", pg.Password, "the password placeholder is read from the environment")
	assert.Equal(t, 5432, pg.Port)
	assert.Equal(t, "portal_backend", pg.Name)
	assert.Equal(t, "require", pg.SSLMode)
	assert.Equal(t, 25, pg.Pool.MaxOpenConns)
	assert.Equal(t, 5, pg.Pool.MaxIdleConns)
	assert.Equal(t, 3600, pg.Pool.MaxConnLifetimeSeconds)
}

func TestLoadConfigReadsEverySetting(t *testing.T) {
	path := writeConfig(t, `
server:
  port: 9083
  timeout: 3s
environment: staging
logging:
  level: warn
  format: text
cors:
  allowedOrigins:
    - https://member.example
    - https://admin.example
  allowedMethods: [GET, PUT]
  allowedHeaders: [Authorization]
  allowCredentials: false
  maxAge: 600
idp:
  jwksURL: https://idp.example/oauth2/jwks
  issuer: https://idp.example
  audience: http://pb.example
  insecureSkipTLSVerify: true
  management:
    baseURL: https://idp.example
    clientID: PORTAL_BACKEND
    clientSecret: client-secret
    scopes: [system, extra]
    resource: https://idp.example/mcp
    dataConsumerRoleID: role-1
pdp:
  url: https://pdp.example
database:
  driver: postgres
  postgres:
    host: db.internal
    port: 6543
    user: pb
    password: pw
    name: portal
    sslMode: verify-full
    pool:
      maxOpenConns: 50
      maxIdleConns: 10
      maxConnLifetimeSeconds: 600
  idleTimeout: 5m
otel:
  endpoint: http://otel-collector:4318
  serviceName: pb-staging
  headers:
    Authorization: Basic b3RlbA==
`)

	cfg, err := loadConfig([]string{"-config", path})
	require.NoError(t, err)

	assert.Equal(t, ServerConfig{Port: 9083, Timeout: 3 * time.Second}, cfg.Server)
	assert.Equal(t, "staging", cfg.Environment)
	assert.Equal(t, logging.Config{Level: "warn", Format: "text"}, cfg.Logging)
	assert.Equal(t, cors.Config{
		AllowedOrigins:   []string{"https://member.example", "https://admin.example"},
		AllowedMethods:   []string{"GET", "PUT"},
		AllowedHeaders:   []string{"Authorization"},
		AllowCredentials: false,
		MaxAge:           600,
	}, cfg.CORS, "lists in the file replace the defaults")
	assert.Equal(t, IDPConfig{
		Config: authn.Config{
			JWKSURL:               "https://idp.example/oauth2/jwks",
			Issuer:                "https://idp.example",
			Audience:              "http://pb.example",
			InsecureSkipTLSVerify: true,
		},
		Management: thunderid.Config{
			BaseURL:            "https://idp.example",
			ClientID:           "PORTAL_BACKEND",
			ClientSecret:       "client-secret",
			Scopes:             []string{"system", "extra"},
			Resource:           "https://idp.example/mcp",
			DataConsumerRoleID: "role-1",
		},
	}, cfg.IDP)
	assert.Equal(t, PDPConfig{URL: "https://pdp.example"}, cfg.PDP)
	assert.Equal(t, telemetry.Config{
		Endpoint:    "http://otel-collector:4318",
		ServiceName: "pb-staging",
		Headers:     map[string]string{"Authorization": "Basic b3RlbA=="},
	}, cfg.Otel)
	assert.Equal(t, 5*time.Minute, cfg.Database.IdleTimeout)

	pg := cfg.Database.Postgres
	assert.Equal(t, 6543, pg.Port)
	assert.Equal(t, "portal", pg.Name)
	assert.Equal(t, "verify-full", pg.SSLMode)
	assert.Equal(t, 50, pg.Pool.MaxOpenConns)
	assert.Equal(t, 10, pg.Pool.MaxIdleConns)
	assert.Equal(t, 600, pg.Pool.MaxConnLifetimeSeconds)
}

func TestLoadConfigPathFromEnvironment(t *testing.T) {
	t.Setenv("TEST_PB_DB_PASSWORD", "s3cret")
	t.Setenv("CONFIG_PATH", writeConfig(t, minimalConfig))

	cfg, err := loadConfig(nil)
	require.NoError(t, err)
	assert.Equal(t, "db.internal", cfg.Database.Postgres.Host)
}

func TestLoadConfigFlagOverridesEnvironment(t *testing.T) {
	t.Setenv("TEST_PB_DB_PASSWORD", "s3cret")
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
	pw := map[string]string{"TEST_PB_DB_PASSWORD": "pw"}

	tests := []struct {
		name    string
		content string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "unset password variable",
			content: minimalConfig,
			wantErr: "TEST_PB_DB_PASSWORD",
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
			name:    "wildcard origin with credentials",
			content: minimalConfig + "cors:\n  allowedOrigins: [\"*\"]\n",
			env:     pw,
			wantErr: "invalid cors configuration",
		},
		{
			name:    "no allowed origins",
			content: minimalConfig + "cors:\n  allowedOrigins: []\n",
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
			content: replace("  audience: http://pb.example\n", ""),
			env:     pw,
			wantErr: "audience is required",
		},
		{
			name:    "missing management base URL",
			content: replace("    baseURL: https://idp.example\n", ""),
			env:     pw,
			wantErr: "idp.management.baseURL",
		},
		{
			name:    "management base URL without scheme",
			content: replace("    baseURL: https://idp.example\n", "    baseURL: idp.example\n"),
			env:     pw,
			wantErr: "idp.management.baseURL",
		},
		{
			name:    "missing management client ID",
			content: replace("    clientID: PORTAL_BACKEND\n", ""),
			env:     pw,
			wantErr: "idp.management.clientID is required",
		},
		{
			name:    "missing management client secret",
			content: replace("    clientSecret: client-secret\n", ""),
			env:     pw,
			wantErr: "idp.management.clientSecret is required",
		},
		{
			name:    "missing PDP URL",
			content: replace("  url: http://pdp.internal:8082\n", ""),
			env:     pw,
			wantErr: "pdp.url",
		},
		{
			name:    "PDP URL without scheme",
			content: replace("  url: http://pdp.internal:8082\n", "  url: pdp.internal:8082\n"),
			env:     pw,
			wantErr: "pdp.url",
		},
		{
			name:    "otel endpoint that is not http",
			content: minimalConfig + "otel:\n  endpoint: ftp://otel.internal\n",
			env:     pw,
			wantErr: "invalid otel configuration",
		},
		{
			name:    "otel export without a service name",
			content: minimalConfig + "otel:\n  endpoint: http://otel.internal:4318\n  serviceName: \"\"\n",
			env:     pw,
			wantErr: "ServiceName is required",
		},
		{
			name:    "missing database host",
			content: replace("    host: db.internal\n", ""),
			env:     pw,
			wantErr: "postgres host is required",
		},
		{
			name:    "missing database password",
			content: replace("    password: \"{{env:TEST_PB_DB_PASSWORD}}\"\n", ""),
			wantErr: "postgres password is required",
		},
		{
			name:    "unsupported driver",
			content: replace("database:\n", "database:\n  driver: sqlite\n  sqlite:\n    path: pb.db\n"),
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
			t.Setenv("TEST_PB_DB_PASSWORD", "")
			require.NoError(t, os.Unsetenv("TEST_PB_DB_PASSWORD"))
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
	t.Setenv("DB_NAME_PB", "portal_backend")
	t.Setenv("IDP_PUBLIC_URL", "http://localhost:8090")
	t.Setenv("PB_AUDIENCE", "http://pb.openndx.local")
	t.Setenv("PORTAL_BACKEND_CLIENT_SECRET", "1234")

	for _, path := range []string{"config.example.yaml", "config.docker.yaml"} {
		t.Run(path, func(t *testing.T) {
			_, err := loadConfig([]string{"-config", path})
			require.NoError(t, err)
		})
	}
}

func TestCommittedOtelExamplesLoad(t *testing.T) {
	t.Setenv("DB_USER", "postgres")
	t.Setenv("DB_PASSWORD", "pw")
	t.Setenv("DB_NAME_PB", "portal_backend")
	t.Setenv("IDP_PUBLIC_URL", "http://localhost:8090")
	t.Setenv("PB_AUDIENCE", "http://pb.openndx.local")
	t.Setenv("PORTAL_BACKEND_CLIENT_SECRET", "1234")

	for _, path := range []string{"config.example.yaml", "config.docker.yaml"} {
		t.Run(path, func(t *testing.T) {
			cfg, err := loadConfig([]string{"-config", withOtelExample(t, path)})
			require.NoError(t, err)
			assert.True(t, cfg.Otel.Enabled(), "the example turns export on")
			assert.Equal(t, "portal-backend", cfg.Otel.ServiceName)
		})
	}
}
