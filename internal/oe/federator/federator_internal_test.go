package federator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/configs"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/internals/errors"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSchemaService is a mock implementation of the schema service
type MockSchemaService struct {
	GetActiveSchemaFunc func() interface{}
}

func (m *MockSchemaService) GetActiveSchema() interface{} {
	if m.GetActiveSchemaFunc != nil {
		return m.GetActiveSchemaFunc()
	}
	return nil
}

// MockSchemaServiceWithSignature is a mock implementation of the schema service with correct signature for reflection
type MockSchemaServiceWithSignature struct {
	SDL string
}

type MockSchemaRecord struct {
	SDL string
}

func (m *MockSchemaServiceWithSignature) GetActiveSchema() (*MockSchemaRecord, error) {
	return &MockSchemaRecord{SDL: m.SDL}, nil
}

func TestFederateQuery_WithMockSchema(t *testing.T) {
	// 1. Mock Provider
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := graphql.Response{
			Data: map[string]interface{}{
				"person": map[string]interface{}{
					"fullName": "John Doe",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer providerServer.Close()

	// 2. Mock PDP
	pdpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := policy.PdpResponse{
			AppAuthorized:           true,
			AppRequiresOwnerConsent: false,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer pdpServer.Close()

	// 3. Setup Config
	cfg := &configs.Config{
		Environment:           "test",
		TrustUpstream:         true, // Trust upstream to avoid JWT validation requirements
		FoundationalIdArgName: "nic",
		Providers: []*configs.ProviderConfig{
			{
				ProviderKey: "drp",
				ProviderURL: providerServer.URL,
				SchemaID:    "drp-schema",
			},
		},
		PdpConfig: configs.PdpConfig{
			ClientURL: pdpServer.URL,
		},
		ArgMapping: []*graphql.ArgMapping{
			{
				ProviderKey:   "drp",
				SchemaID:      "drp-schema",
				TargetArgName: "nic",
				SourceArgPath: "personInfo-nic",
				TargetArgPath: "person",
			},
		},
	}

	// 4. Setup Federator
	providerHandler := provider.NewProviderHandler(nil)

	schemaSDL := `
		directive @sourceInfo(providerKey: String!, providerField: String!, schemaId: String) on FIELD_DEFINITION
		type Query {
			personInfo(nic: String!): PersonInfo @sourceInfo(providerKey: "drp", providerField: "person", schemaId: "drp-schema")
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerKey: "drp", providerField: "person.fullName", schemaId: "drp-schema")
		}
	`

	mockService := &MockSchemaServiceWithSignature{SDL: schemaSDL}
	f, err := Initialize(context.Background(), cfg, providerHandler, mockService)
	if err != nil {
		t.Fatalf("Failed to initialize federator: %v", err)
	}

	// 5. Execute Query
	req := graphql.Request{
		Query: `query {
			personInfo(nic: "123") {
				fullName
			}
		}`,
	}

	ctx := context.Background()
	resp := f.FederateQuery(ctx, req, &auth.ConsumerAssertion{
		Subscriber: "sub-123",
		ClientID:   "app-123",
	})

	// 6. Assertions
	require.Empty(t, resp.Errors)
	require.NotNil(t, resp.Data)

	data := resp.Data
	personInfo, ok := data["personInfo"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "John Doe", personInfo["fullName"])
}

// TestFederateQuery_DataOwnerIdentification verifies that the Data Owner ID is
// matched by the configured foundationalIdArgName rather than by argument
// position, and that multiple mappings agreeing on the same value don't
// trigger a false conflict.
//
// Note: the AMBIGUOUS_IDENTIFIER path (foundationalIdValues containing two
// different values) is not covered here because it isn't currently reachable
// through a real query: ExtractRequiredArguments (arghandler.go) dedupes by
// ArgMapping pointer, so the first actual argument that matches a given
// mapping "claims" it for the rest of the request, and every ArgSource for
// that mapping ends up holding that same first-seen value.
func TestFederateQuery_DataOwnerIdentification(t *testing.T) {
	// sessionInfo's "requestId" argument is extracted before personInfo's "nic"
	// argument, so a positional implementation would pick the wrong value.
	// personInfo's "nic" argument is also matched by two separate mappings
	// (one targeting "person", one targeting the "person.fullName" subfield),
	// producing two equal foundationalIdValues from a single query argument.
	schemaSDL := `
		directive @sourceInfo(providerKey: String!, providerField: String!, schemaId: String) on FIELD_DEFINITION
		type Query {
			personInfo(nic: String!): PersonInfo @sourceInfo(providerKey: "drp", providerField: "person", schemaId: "drp-schema")
			sessionInfo(requestId: String!): SessionInfo @sourceInfo(providerKey: "drp", providerField: "session", schemaId: "drp-schema")
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerKey: "drp", providerField: "person.fullName", schemaId: "drp-schema")
		}
		type SessionInfo {
			status: String @sourceInfo(providerKey: "drp", providerField: "session.status", schemaId: "drp-schema")
		}
	`
	argMapping := []*graphql.ArgMapping{
		{ProviderKey: "drp", SchemaID: "drp-schema", TargetArgName: "nic", SourceArgPath: "personInfo-nic", TargetArgPath: "person"},
		{ProviderKey: "drp", SchemaID: "drp-schema", TargetArgName: "nic", SourceArgPath: "fullName-nic", TargetArgPath: "person.fullName"},
		{ProviderKey: "drp", SchemaID: "drp-schema", TargetArgName: "requestId", SourceArgPath: "sessionInfo-requestId", TargetArgPath: "session"},
	}

	// setupFederator wires a Federator whose PDP mock always requires consent
	// and whose CE mock captures the OwnerID it receives, then rejects the
	// consent. This lets the test assert on the resolved Data Owner ID
	// without needing a full multi-provider round trip.
	setupFederator := func(t *testing.T) (*Federator, *string) {
		var capturedOwnerID string

		pdpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := policy.PdpResponse{
				AppAuthorized:           true,
				AppRequiresOwnerConsent: true,
				ConsentRequiredFields: []policy.ConsentRequiredField{
					{FieldName: "fullName", SchemaID: "drp-schema"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		t.Cleanup(pdpServer.Close)

		ceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req consent.CreateConsentRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			capturedOwnerID = req.ConsentRequirement.OwnerID

			resp := consent.ConsentResponseInternalView{
				ConsentID: "consent-1",
				Status:    "pending",
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resp)
		}))
		t.Cleanup(ceServer.Close)

		cfg := &configs.Config{
			Environment:           "test",
			TrustUpstream:         true,
			FoundationalIdArgName: "nic",
			PdpConfig:             configs.PdpConfig{ClientURL: pdpServer.URL},
			CeConfig:              configs.CeConfig{ClientURL: ceServer.URL},
			ArgMapping:            argMapping,
		}

		providerHandler := provider.NewProviderHandler(nil)
		mockService := &MockSchemaServiceWithSignature{SDL: schemaSDL}

		f, err := Initialize(context.Background(), cfg, providerHandler, mockService)
		require.NoError(t, err)

		return f, &capturedOwnerID
	}

	assertion := &auth.ConsumerAssertion{Subscriber: "sub-123", ClientID: "app-123"}

	t.Run("matches identifier by name, not position", func(t *testing.T) {
		f, capturedOwnerID := setupFederator(t)

		req := graphql.Request{
			Query: `query {
				sessionInfo(requestId: "req-999") { status }
				personInfo(nic: "123") { fullName }
			}`,
		}

		resp := f.FederateQuery(context.Background(), req, assertion)

		require.NotEmpty(t, resp.Errors)
		extensions := resp.Errors[0].(map[string]interface{})["extensions"].(map[string]interface{})
		assert.Equal(t, errors.CodeCENotApproved, extensions["code"])
		assert.Equal(t, "123", *capturedOwnerID)
	})

	t.Run("allows repeated equal identifiers", func(t *testing.T) {
		f, capturedOwnerID := setupFederator(t)

		// The single "nic" argument below is matched by both the "person" and
		// "person.fullName" mappings, producing two equal foundationalIdValues.
		req := graphql.Request{
			Query: `query {
				personInfo(nic: "123") { fullName }
			}`,
		}

		resp := f.FederateQuery(context.Background(), req, assertion)

		require.NotEmpty(t, resp.Errors)
		extensions := resp.Errors[0].(map[string]interface{})["extensions"].(map[string]interface{})
		assert.Equal(t, errors.CodeCENotApproved, extensions["code"])
		assert.Equal(t, "123", *capturedOwnerID)
	})
}

func TestFederateQuery_PDPDeny(t *testing.T) {
	// Mock PDP to deny
	pdpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := policy.PdpResponse{
			AppAuthorized: false,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer pdpServer.Close()

	cfg := &configs.Config{
		Environment:           "test",
		TrustUpstream:         true, // Trust upstream to avoid JWT validation requirements
		FoundationalIdArgName: "nic",
		PdpConfig: configs.PdpConfig{
			ClientURL: pdpServer.URL,
		},
		ArgMapping: []*graphql.ArgMapping{
			{
				ProviderKey:   "drp",
				SchemaID:      "drp-schema",
				TargetArgName: "nic",
				SourceArgPath: "personInfo-nic",
				TargetArgPath: "person",
			},
		},
	}

	providerHandler := provider.NewProviderHandler(nil)

	schemaSDL := `
		directive @sourceInfo(providerKey: String!, providerField: String!, schemaId: String) on FIELD_DEFINITION
		type Query {
			personInfo(nic: String!): PersonInfo @sourceInfo(providerKey: "drp", providerField: "person", schemaId: "drp-schema")
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerKey: "drp", providerField: "person.fullName", schemaId: "drp-schema")
		}
	`
	mockService := &MockSchemaServiceWithSignature{SDL: schemaSDL}
	f, err := Initialize(context.Background(), cfg, providerHandler, mockService)
	if err != nil {
		t.Fatalf("Failed to initialize federator: %v", err)
	}

	req := graphql.Request{
		Query: `query { personInfo(nic: "123") { fullName } }`,
	}
	consumerInfo := &auth.ConsumerAssertion{
		Subscriber: "sub-123",
		ClientID:   "app-123",
	}

	resp := f.FederateQuery(context.Background(), req, consumerInfo)

	require.NotEmpty(t, resp.Errors)
	// Check for specific error message or code if possible
	// The code returns: "Access denied"
	assert.Contains(t, resp.Errors[0].(map[string]interface{})["message"], "Access denied")
}

// TestInitialize_FailsWithInvalidConfig tests that Initialize fails fast when
// trustUpstream is false but JWT configuration is invalid
func TestInitialize_FailsWithInvalidConfig(t *testing.T) {
	providerHandler := provider.NewProviderHandler(nil)

	t.Run("fails when trustUpstream=false and JWKS URL is empty", func(t *testing.T) {
		cfg := &configs.Config{
			Environment:   "production",
			TrustUpstream: false,
			JWT: configs.JWTConfig{
				JwksUrl: "", // Missing JWKS URL
			},
		}

		_, err := Initialize(context.Background(), cfg, providerHandler, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trustUpstream is false")
		assert.Contains(t, err.Error(), "JwksUrl is not configured")
	})

	t.Run("fails when trustUpstream=false and JWKS URL is invalid", func(t *testing.T) {
		cfg := &configs.Config{
			Environment:   "production",
			TrustUpstream: false,
			JWT: configs.JWTConfig{
				JwksUrl: "http://invalid-url:99999/jwks", // Invalid URL
			},
		}

		_, err := Initialize(context.Background(), cfg, providerHandler, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize TokenValidator")
	})

	t.Run("succeeds when trustUpstream=true even without JWKS URL", func(t *testing.T) {
		cfg := &configs.Config{
			Environment:           "production",
			TrustUpstream:         true,
			FoundationalIdArgName: "nic",
			JWT: configs.JWTConfig{
				JwksUrl: "", // Missing JWKS URL is OK when trusting upstream
			},
		}

		_, err := Initialize(context.Background(), cfg, providerHandler, nil)
		require.NoError(t, err)
	})

	t.Run("succeeds when trustUpstream=true even with invalid JWKS URL", func(t *testing.T) {
		cfg := &configs.Config{
			Environment:           "production",
			TrustUpstream:         true,
			FoundationalIdArgName: "nic",
			JWT: configs.JWTConfig{
				JwksUrl: "http://invalid-url:99999/jwks", // Invalid URL is logged but not fatal
			},
		}

		_, err := Initialize(context.Background(), cfg, providerHandler, nil)
		require.NoError(t, err) // Should not fail, just log warning
	})

	t.Run("fails when foundationalIdArgName is not configured", func(t *testing.T) {
		cfg := &configs.Config{
			Environment:   "production",
			TrustUpstream: true,
		}

		_, err := Initialize(context.Background(), cfg, providerHandler, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "foundationalIdArgName is not configured")
	})
}

func TestInitialize_SkipsProvidersWithEmptyKey(t *testing.T) {
	providerHandler := provider.NewProviderHandler(nil)
	cfg := &configs.Config{
		Environment:           "production",
		TrustUpstream:         true,
		FoundationalIdArgName: "nic",
		Providers: []*configs.ProviderConfig{
			{ProviderKey: "", ProviderURL: "http://empty-key.example", SchemaID: "schema1"},
			{ProviderKey: "ok", ProviderURL: "http://ok.example", SchemaID: "schema2"},
		},
	}

	_, err := Initialize(context.Background(), cfg, providerHandler, nil)
	require.NoError(t, err)

	_, exists := providerHandler.GetProvider("", "schema1")
	assert.False(t, exists)

	p, exists := providerHandler.GetProvider("ok", "schema2")
	require.True(t, exists)
	assert.Equal(t, "http://ok.example", p.ServiceUrl)
}
