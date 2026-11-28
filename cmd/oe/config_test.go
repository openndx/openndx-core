// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	remoteauth "github.com/OpenNSW/core/remote/auth"
	"github.com/OpenNSW/core/telemetry"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/logging"
	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/federator"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalConfig holds only the settings that have no default.
const minimalConfig = `
idp:
  jwksURL: https://idp.example/oauth2/jwks
  issuer: https://idp.example
  audience: http://api.example
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
	path := writeConfig(t, minimalConfig)

	cfg, err := loadConfig([]string{"-config", path})
	require.NoError(t, err)

	assert.Equal(t, ServerConfig{Port: 4000, Timeout: 30 * time.Second}, cfg.Server)
	assert.Empty(t, cfg.Environment)
	assert.Equal(t, logging.Config{Level: "info", Format: "json"}, cfg.Logging)
	assert.Equal(t, filepath.Join(filepath.Dir(path), "schema.graphql"), cfg.SchemaPath,
		"the schema defaults to the schema.graphql next to the config file")
	assert.Equal(t, authn.Config{
		JWKSURL:  "https://idp.example/oauth2/jwks",
		Issuer:   "https://idp.example",
		Audience: "http://api.example",
	}, cfg.IDP)
	assert.Empty(t, cfg.PDP.URL, "the PDP check is off by default")
	assert.Equal(t, ConsentEngineConfig{}, cfg.ConsentEngine, "the consent check is off by default")
	assert.Equal(t, AuditConfig{ActorType: "SERVICE", ActorID: "orchestration-engine"}, cfg.Audit)
	assert.Equal(t, telemetry.Config{ServiceName: "orchestration-engine"}, cfg.Otel, "export, and so audit, is off by default")
	assert.Empty(t, cfg.Providers)
	assert.Empty(t, cfg.ArgMappings)
}

func TestLoadConfigReadsEverySetting(t *testing.T) {
	t.Setenv("TEST_OE_OTEL_AUTH", "Basic b3RlbA==")
	t.Setenv("TEST_OE_CIVIL_REGISTRY_SECRET", "civil-registry-secret")
	// Absolute on every OS; on Windows that needs a drive letter.
	absSchemaPath := filepath.Join(t.TempDir(), "schema.graphql")

	cfg, err := loadConfig([]string{"-config", writeConfig(t, `
server:
  port: 4100
  timeout: 45s
environment: staging
logging:
  level: debug
  format: text
schemaPath: `+absSchemaPath+`
idp:
  jwksURL: https://idp.example/oauth2/jwks
  issuer: https://idp.example
  audience: http://api.example
  insecureSkipTLSVerify: true
pdp:
  url: http://pdp.internal:8082
consentEngine:
  url: http://ce.internal:8081/internal/api/v1
  ownerIdArgName: nic
audit:
  actorType: SERVICE
  actorId: oe-1
providers:
  - id: identity-registry-v1
    url: http://identity-registry.internal/graphql
    timeout: 5s
    auth:
      type: apiKey
      apiKey:
        key: X-API-Key
        value: identity-registry-key
  - id: civil-registry-v1
    url: http://civil-registry.internal/graphql
    auth:
      type: oauth2
      oauth2:
        tokenURL: http://civil-registry.internal/oauth/token
        clientID: oe
        clientSecret: "{{env:TEST_OE_CIVIL_REGISTRY_SECRET}}"
argMappings:
  - schemaId: identity-registry-v1
    targetArgName: nic
    sourceArgPath: personInfo-nic
    targetArgPath: person
otel:
  endpoint: http://otel-collector:4318
  logs:
    endpoint: http://logs.internal:4318/v1/logs
  serviceName: oe-staging
  headers:
    Authorization: "{{env:TEST_OE_OTEL_AUTH}}"
`)})
	require.NoError(t, err)

	assert.Equal(t, ServerConfig{Port: 4100, Timeout: 45 * time.Second}, cfg.Server)
	assert.Equal(t, "staging", cfg.Environment)
	assert.Equal(t, logging.Config{Level: "debug", Format: "text"}, cfg.Logging)
	assert.Equal(t, absSchemaPath, cfg.SchemaPath, "an absolute schemaPath is used as is")
	assert.True(t, cfg.IDP.InsecureSkipTLSVerify)
	assert.Equal(t, "http://pdp.internal:8082", cfg.PDP.URL)
	assert.Equal(t, ConsentEngineConfig{URL: "http://ce.internal:8081/internal/api/v1", OwnerIDArgName: "nic"}, cfg.ConsentEngine)
	assert.Equal(t, AuditConfig{ActorType: "SERVICE", ActorID: "oe-1"}, cfg.Audit)
	assert.Equal(t, telemetry.Config{
		Endpoint:    "http://otel-collector:4318",
		Logs:        telemetry.SignalConfig{Endpoint: "http://logs.internal:4318/v1/logs"},
		ServiceName: "oe-staging",
		Headers:     map[string]string{"Authorization": "Basic b3RlbA=="},
	}, cfg.Otel)

	require.Len(t, cfg.Providers, 2)
	assert.Equal(t, provider.Config{
		ID:      "identity-registry-v1",
		URL:     "http://identity-registry.internal/graphql",
		Timeout: "5s",
		Auth: &remoteauth.Config{
			Type:   remoteauth.TypeAPIKey,
			APIKey: &remoteauth.APIKeyConfig{Key: "X-API-Key", Value: "identity-registry-key"},
		},
	}, cfg.Providers[0])
	require.NotNil(t, cfg.Providers[1].Auth)
	require.NotNil(t, cfg.Providers[1].Auth.OAuth2)
	assert.Equal(t, "civil-registry-secret", cfg.Providers[1].Auth.OAuth2.ClientSecret, "the secret placeholder is read from the environment")

	assert.Equal(t, []*graphql.ArgMapping{{
		SchemaID:      "identity-registry-v1",
		TargetArgName: "nic",
		SourceArgPath: "personInfo-nic",
		TargetArgPath: "person",
	}}, cfg.ArgMappings)
}

func TestLoadConfigPathFromEnvironment(t *testing.T) {
	t.Setenv("CONFIG_PATH", writeConfig(t, minimalConfig))

	cfg, err := loadConfig(nil)
	require.NoError(t, err)
	assert.Equal(t, "http://api.example", cfg.IDP.Audience)
}

func TestLoadConfigFlagOverridesEnvironment(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))

	cfg, err := loadConfig([]string{"-config", writeConfig(t, minimalConfig)})
	require.NoError(t, err)
	assert.Equal(t, "http://api.example", cfg.IDP.Audience)
}

func TestLoadConfigDefaultPath(t *testing.T) {
	t.Setenv("CONFIG_PATH", "")
	assert.Equal(t, "config.yaml", configPathFromEnv())
}

func TestLoadConfigHelp(t *testing.T) {
	_, err := loadConfig([]string{"-h"})
	assert.True(t, errors.Is(err, flag.ErrHelp))
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := loadConfig([]string{"-config", filepath.Join(t.TempDir(), "missing.yaml")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		replace [2]string
		wantErr string
	}{
		{name: "port out of range", extra: "server:\n  port: 70000", wantErr: "server.port"},
		{name: "non-positive timeout", extra: "server:\n  timeout: 0s", wantErr: "server.timeout must be positive"},
		{name: "unknown log level", extra: "logging:\n  level: verbose", wantErr: "logging.level"},
		{name: "unknown log format", extra: "logging:\n  format: xml", wantErr: "logging.format"},
		{name: "empty schemaPath", extra: `schemaPath: ""`, wantErr: "schemaPath must not be empty"},
		{name: "consent engine without ownerIdArgName", extra: "consentEngine:\n  url: http://ce.internal/internal/api/v1", wantErr: "consentEngine.ownerIdArgName is required when consentEngine.url is set"},
		{name: "missing idp audience", replace: [2]string{"  audience: http://api.example", ""}, wantErr: "invalid idp configuration"},
		{name: "invalid pdp url", extra: "pdp:\n  url: pdp.internal", wantErr: "pdp.url"},
		{name: "invalid consent engine url", extra: "consentEngine:\n  url: ftp://ce.internal\n  ownerIdArgName: nic", wantErr: "consentEngine.url must use http or https"},
		{name: "empty audit actor", extra: "audit:\n  actorId: \"\"", wantErr: "audit.actorType and audit.actorId"},
		{name: "incomplete arg mapping", extra: "argMappings:\n  - schemaId: identity-registry-v1\n    targetArgName: nic", wantErr: "argMappings[0] must set"},
		{
			name:    "arg mapping for an unknown provider",
			extra:   "providers:\n  - id: identity-registry-v1\n    url: http://identity-registry.internal\nargMappings:\n  - schemaId: identity-registy-v1\n    targetArgName: nic\n    sourceArgPath: personInfo-nic\n    targetArgPath: person",
			wantErr: `argMappings[0].schemaId "identity-registy-v1" matches no provider id`,
		},
		{name: "invalid otel endpoint", extra: "otel:\n  endpoint: otel-collector:4318", wantErr: "invalid otel configuration"},
		{name: "otel export without a service name", extra: "otel:\n  endpoint: http://otel-collector:4318\n  serviceName: \"\"", wantErr: "ServiceName is required"},
		{name: "unset placeholder", extra: "otel:\n  headers:\n    Authorization: \"{{env:TEST_OE_UNSET_VARIABLE}}\"", wantErr: "TEST_OE_UNSET_VARIABLE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := minimalConfig
			if tt.replace[0] != "" {
				content = strings.Replace(content, tt.replace[0], tt.replace[1], 1)
			}
			_, err := loadConfig([]string{"-config", writeConfig(t, content+tt.extra+"\n")})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCommittedConfigsLoad(t *testing.T) {
	t.Setenv("IDP_PUBLIC_URL", "http://localhost:8090")
	t.Setenv("IDP_AUDIENCE", "http://api.openndx.local")

	for _, path := range []string{"config.example.yaml", "config.docker.yaml", "../../tests/integration/oe-config.yaml"} {
		t.Run(path, func(t *testing.T) {
			cfg, err := loadConfig([]string{"-config", path})
			require.NoError(t, err)
			_, err = provider.NewRegistry(cfg.Providers...)
			require.NoError(t, err, "the providers are valid")
			_, err = schema.Load(cfg.SchemaPath)
			require.NoError(t, err, "schemaPath names the schema next to the config")
		})
	}
}

// TestCommittedConfigsFederate exercises the committed configs and schema so the
// walkthrough's owner ID reaches each selected provider.
func TestCommittedConfigsFederate(t *testing.T) {
	t.Setenv("IDP_PUBLIC_URL", "http://localhost:8090")
	t.Setenv("IDP_AUDIENCE", "http://api.openndx.local")

	for _, configFile := range []string{"config.docker.yaml", "config.example.yaml"} {
		for _, tc := range []struct {
			name      string
			request   graphql.Request
			providers []string
			want      map[string]interface{}
		}{
			{
				name:      "public Identity Registry literal",
				request:   graphql.Request{Query: `query { personInfo(nic: "FL-4829105736") { fullName } }`},
				providers: []string{"identity-registry-v1"},
				want:      map[string]interface{}{"fullName": "Suresh Fernando"},
			},
			{
				name: "public Identity Registry variable",
				request: graphql.Request{
					Query:     `query Person($nic: String!) { personInfo(nic: $nic) { fullName } }`,
					Variables: map[string]interface{}{"nic": "FL-4829105736"},
				},
				providers: []string{"identity-registry-v1"},
				want:      map[string]interface{}{"fullName": "Suresh Fernando"},
			},
			{
				name: "Civil Registry variable",
				request: graphql.Request{
					Query:     `query Person($nic: String!) { personInfo(nic: $nic) { birthInfo { district } } }`,
					Variables: map[string]interface{}{"nic": "FL-4829105736"},
				},
				providers: []string{"civil-registry-v1"},
				want:      map[string]interface{}{"birthInfo": map[string]interface{}{"district": "Central"}},
			},
			{
				name:      "both providers",
				request:   graphql.Request{Query: `query { personInfo(nic: "FL-4829105736") { fullName profession birthInfo { birthPlace district } } }`},
				providers: []string{"identity-registry-v1", "civil-registry-v1"},
				want: map[string]interface{}{
					"fullName": "Suresh Fernando", "profession": "Software Engineer",
					"birthInfo": map[string]interface{}{"birthPlace": "Central General Hospital", "district": "Central"},
				},
			},
		} {
			t.Run(configFile+"/"+tc.name, func(t *testing.T) {
				cfg, err := loadConfig([]string{"-config", configFile})
				require.NoError(t, err)
				schemas, err := schema.Load(cfg.SchemaPath)
				require.NoError(t, err)

				pdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = json.NewEncoder(w).Encode(policy.PdpResponse{AppAuthorized: true})
				}))
				t.Cleanup(pdp.Close)

				calls := make(chan string, 2)
				for i := range cfg.Providers {
					p := &cfg.Providers[i]
					id := p.ID
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var request graphql.Request
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Errorf("decode %s request: %v", id, err)
							http.Error(w, "invalid request", http.StatusBadRequest)
							return
						}
						if !assert.Contains(t, request.Query, `(nic: $nic)`) || !assert.Equal(t, "FL-4829105736", request.Variables["nic"]) {
							http.Error(w, "missing nic", http.StatusBadRequest)
							return
						}
						calls <- id
						var data map[string]interface{}
						if id == "identity-registry-v1" {
							data = map[string]interface{}{"person": map[string]interface{}{"fullName": "Suresh Fernando", "profession": "Software Engineer"}}
						} else {
							data = map[string]interface{}{"getPersonInfo": map[string]interface{}{"birthPlace": "Central General Hospital", "district": "Central"}}
						}
						_ = json.NewEncoder(w).Encode(graphql.Response{Data: data})
					}))
					t.Cleanup(server.Close)
					p.URL = server.URL
					p.Auth = nil
				}

				registry, err := provider.NewRegistry(cfg.Providers...)
				require.NoError(t, err)
				f, err := federator.New(federator.Deps{
					Providers: federator.ProviderLookup(registry.Get),
					Schemas:   schemas,
					Policy:    policy.NewPdpClient(pdp.URL),
				}, federator.Options{
					OwnerIDArgName: cfg.ConsentEngine.OwnerIDArgName,
					ArgMapping:     cfg.ArgMappings,
				})
				require.NoError(t, err)

				response := f.FederateQuery(context.Background(), tc.request, &auth.ConsumerAssertion{ApplicationID: "demo-client"})
				require.Empty(t, response.Errors)
				require.Equal(t, tc.want, response.Data["personInfo"])
				require.Len(t, calls, len(tc.providers))
				got := make([]string, 0, len(tc.providers))
				for range tc.providers {
					got = append(got, <-calls)
				}
				require.ElementsMatch(t, tc.providers, got)
			})
		}
	}
}

func TestCommittedOtelExamplesLoad(t *testing.T) {
	t.Setenv("IDP_PUBLIC_URL", "http://localhost:8090")
	t.Setenv("IDP_AUDIENCE", "http://api.openndx.local")

	for _, path := range []string{"config.example.yaml", "config.docker.yaml"} {
		t.Run(path, func(t *testing.T) {
			cfg, err := loadConfig([]string{"-config", withOtelExample(t, path)})
			require.NoError(t, err)
			assert.True(t, cfg.Otel.Enabled(), "the example turns export on")
			assert.Equal(t, "orchestration-engine", cfg.Otel.ServiceName)
		})
	}
}
