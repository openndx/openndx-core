package federator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/auth"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/configs"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/policy"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/provider"
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
		Environment:   "test",
		TrustUpstream: true, // Trust upstream to avoid JWT validation requirements
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
		Environment:   "test",
		TrustUpstream: true, // Trust upstream to avoid JWT validation requirements
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
			Environment:   "production",
			TrustUpstream: true,
			JWT: configs.JWTConfig{
				JwksUrl: "", // Missing JWKS URL is OK when trusting upstream
			},
		}

		_, err := Initialize(context.Background(), cfg, providerHandler, nil)
		require.NoError(t, err)
	})

	t.Run("succeeds when trustUpstream=true even with invalid JWKS URL", func(t *testing.T) {
		cfg := &configs.Config{
			Environment:   "production",
			TrustUpstream: true,
			JWT: configs.JWTConfig{
				JwksUrl: "http://invalid-url:99999/jwks", // Invalid URL is logged but not fatal
			},
		}

		_, err := Initialize(context.Background(), cfg, providerHandler, nil)
		require.NoError(t, err) // Should not fail, just log warning
	})
}
