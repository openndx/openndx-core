// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/internals/errors"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer providerServer.Close()

	// 2. Mock PDP
	pdpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := policy.PdpResponse{
			AppAuthorized:           true,
			AppRequiresOwnerConsent: false,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer pdpServer.Close()

	// 3. Setup Config
	cfg := &testConfig{
		OwnerIDArgName: "nic",
		Providers: []provider.Config{
			{
				ID:  "identity-registry-schema",
				URL: providerServer.URL,
			},
		},
		PdpURL: pdpServer.URL,
		ArgMapping: []*graphql.ArgMapping{
			{
				SchemaID:      "identity-registry-schema",
				TargetArgName: "nic",
				SourceArgPath: "personInfo-nic",
				TargetArgPath: "person",
			},
		},
	}

	// 4. Setup Federator
	schemaSDL := `
		directive @sourceInfo(providerField: String!, schemaId: String!) on FIELD_DEFINITION
		type Query {
			personInfo(nic: String!): PersonInfo @sourceInfo(providerField: "person", schemaId: "identity-registry-schema")
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerField: "person.fullName", schemaId: "identity-registry-schema")
		}
	`

	f := newTestFederator(t, cfg, staticSchema(schemaSDL))

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
// matched by the configured ownerIdArgName rather than by argument
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
		directive @sourceInfo(providerField: String!, schemaId: String!) on FIELD_DEFINITION
		type Query {
			personInfo(nic: String!): PersonInfo @sourceInfo(providerField: "person", schemaId: "identity-registry-schema")
			sessionInfo(requestId: String!): SessionInfo @sourceInfo(providerField: "session", schemaId: "identity-registry-schema")
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerField: "person.fullName", schemaId: "identity-registry-schema")
		}
		type SessionInfo {
			status: String @sourceInfo(providerField: "session.status", schemaId: "identity-registry-schema")
		}
	`
	argMapping := []*graphql.ArgMapping{
		{SchemaID: "identity-registry-schema", TargetArgName: "nic", SourceArgPath: "personInfo-nic", TargetArgPath: "person"},
		{SchemaID: "identity-registry-schema", TargetArgName: "nic", SourceArgPath: "fullName-nic", TargetArgPath: "person.fullName"},
		{SchemaID: "identity-registry-schema", TargetArgName: "requestId", SourceArgPath: "sessionInfo-requestId", TargetArgPath: "session"},
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
					{FieldName: "fullName", SchemaID: "identity-registry-schema"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
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
			_ = json.NewEncoder(w).Encode(resp)
		}))
		t.Cleanup(ceServer.Close)

		cfg := &testConfig{
			OwnerIDArgName: "nic",
			PdpURL:         pdpServer.URL,
			CeURL:          ceServer.URL,
			ArgMapping:     argMapping,
		}

		f := newTestFederator(t, cfg, staticSchema(schemaSDL))

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
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer pdpServer.Close()

	cfg := &testConfig{
		OwnerIDArgName: "nic",
		PdpURL:         pdpServer.URL,
		ArgMapping: []*graphql.ArgMapping{
			{
				SchemaID:      "identity-registry-schema",
				TargetArgName: "nic",
				SourceArgPath: "personInfo-nic",
				TargetArgPath: "person",
			},
		},
	}

	schemaSDL := `
		directive @sourceInfo(providerField: String!, schemaId: String!) on FIELD_DEFINITION
		type Query {
			personInfo(nic: String!): PersonInfo @sourceInfo(providerField: "person", schemaId: "identity-registry-schema")
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerField: "person.fullName", schemaId: "identity-registry-schema")
		}
	`
	f := newTestFederator(t, cfg, staticSchema(schemaSDL))

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

// TestNew tests that New fails fast on configuration that would reject every request.
func TestNew(t *testing.T) {
	providers := fakeRegistry{}
	schemas := staticSchema("type Query { a: String }")

	t.Run("fails without a provider registry", func(t *testing.T) {
		_, err := New(Deps{Schemas: schemas}, Options{OwnerIDArgName: "nic"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider registry is required")
	})

	t.Run("fails without a schema source", func(t *testing.T) {
		_, err := New(Deps{Providers: providers}, Options{OwnerIDArgName: "nic"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "schema source is required")
	})

	t.Run("fails when consent is set up without ownerIdArgName", func(t *testing.T) {
		_, err := New(Deps{Providers: providers, Schemas: schemas, Consent: &fakeConsent{}}, Options{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ownerIdArgName is not configured")
	})

	t.Run("succeeds without consent or ownerIdArgName", func(t *testing.T) {
		_, err := New(Deps{Providers: providers, Schemas: schemas}, Options{})
		require.NoError(t, err)
	})

	t.Run("fails when a targetArgType isn't a GraphQL type", func(t *testing.T) {
		_, err := New(Deps{Providers: providers, Schemas: schemas}, Options{
			OwnerIDArgName: "nic",
			ArgMapping:     []*graphql.ArgMapping{{SchemaID: "identity-registry", SourceArgPath: "a-nic", TargetArgPath: "person", TargetArgName: "nic", TargetArgType: "[ID"}},
		})
		assert.EqualError(t, err, `fatal configuration error: argMappings[0].targetArgType: "[ID" is not a GraphQL type`)
	})

	t.Run("succeeds without optional dependencies", func(t *testing.T) {
		f, err := New(Deps{Providers: providers, Schemas: schemas}, Options{OwnerIDArgName: "nic"})
		require.NoError(t, err)
		assert.NotNil(t, f.deps.Auditor, "a nil Auditor should default to one that records nothing")
	})
}

// fakePolicy is an in-memory PolicyDecider that records the request it receives.
type fakePolicy struct {
	resp *policy.PdpResponse
	got  *policy.PdpRequest
}

func (p *fakePolicy) MakePdpRequest(_ context.Context, req *policy.PdpRequest) (*policy.PdpResponse, error) {
	p.got = req
	return p.resp, nil
}

// fakeConsent is an in-memory ConsentRequester that records the request it receives.
type fakeConsent struct {
	resp *consent.ConsentResponseInternalView
	got  *consent.CreateConsentRequest
}

func (c *fakeConsent) CreateConsent(_ context.Context, req *consent.CreateConsentRequest) (*consent.ConsentResponseInternalView, error) {
	c.got = req
	return c.resp, nil
}

// failingSchemaSource is a SchemaSource whose lookup always fails.
type failingSchemaSource struct{}

func (failingSchemaSource) Active(context.Context) (*schema.CompiledSchema, error) {
	return nil, fmt.Errorf("could not find schema.graphql file in any expected location")
}

// fakeProvider is an in-memory ProviderClient that records the request it receives.
type fakeProvider struct {
	resp graphql.Response
	err  error

	mu  sync.Mutex
	got []graphql.Request
}

func (p *fakeProvider) Execute(_ context.Context, req graphql.Request) (graphql.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got = append(p.got, req)
	return p.resp, p.err
}

// fakeRegistry is a ProviderRegistry keyed by schema ID.
type fakeRegistry map[string]*fakeProvider

func (r fakeRegistry) Get(schemaID string) (ProviderClient, bool) {
	p, ok := r[schemaID]
	if !ok {
		return nil, false
	}
	return p, true
}

// recordingAuditor is an Auditor that records provider fetches.
type recordingAuditor struct {
	noopAuditor

	mu      sync.Mutex
	fetches []ProviderFetch
	errs    []error
}

func (a *recordingAuditor) ProviderFetch(_ context.Context, fetch ProviderFetch, _ *graphql.Response, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fetches = append(a.fetches, fetch)
	a.errs = append(a.errs, err)
}

func TestFederateQuery_InjectedDeps(t *testing.T) {
	schemaSDL := `
		directive @sourceInfo(providerField: String!, schemaId: String!) on FIELD_DEFINITION
		type Query {
			personInfo(nic: String!): PersonInfo @sourceInfo(providerField: "person", schemaId: "identity-registry-schema")
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerField: "person.fullName", schemaId: "identity-registry-schema")
		}
	`
	opts := Options{
		OwnerIDArgName: "nic",
		ArgMapping: []*graphql.ArgMapping{
			{SchemaID: "identity-registry-schema", TargetArgName: "nic", SourceArgPath: "personInfo-nic", TargetArgPath: "person"},
		},
	}
	req := graphql.Request{Query: `query { personInfo(nic: "123") { fullName } }`}
	assertion := &auth.ConsumerAssertion{ApplicationID: "app-123"}
	consentRequired := &policy.PdpResponse{
		AppAuthorized:           true,
		AppRequiresOwnerConsent: true,
		ConsentRequiredFields:   []policy.ConsentRequiredField{{FieldName: "person.fullName", SchemaID: "identity-registry-schema"}},
	}

	t.Run("calls the injected policy decider and consent requester", func(t *testing.T) {
		pdp := &fakePolicy{resp: consentRequired}
		ce := &fakeConsent{resp: &consent.ConsentResponseInternalView{ConsentID: "consent-1", Status: "pending"}}
		f, err := New(Deps{
			Providers: fakeRegistry{},
			Schemas:   staticSchema(schemaSDL),
			Policy:    pdp,
			Consent:   ce,
		}, opts)
		require.NoError(t, err)

		resp := f.FederateQuery(context.Background(), req, assertion)

		require.NotEmpty(t, resp.Errors)
		extensions := resp.Errors[0].(map[string]interface{})["extensions"].(map[string]interface{})
		assert.Equal(t, errors.CodeCENotApproved, extensions["code"])
		require.NotNil(t, pdp.got)
		assert.Equal(t, "app-123", pdp.got.AppId)
		require.NotNil(t, ce.got)
		assert.Equal(t, "123", ce.got.ConsentRequirement.OwnerID)
	})

	t.Run("rejects consent-required queries without a consent requester", func(t *testing.T) {
		f, err := New(Deps{
			Providers: fakeRegistry{},
			Schemas:   staticSchema(schemaSDL),
			Policy:    &fakePolicy{resp: consentRequired},
		}, opts)
		require.NoError(t, err)

		resp := f.FederateQuery(context.Background(), req, assertion)

		require.NotEmpty(t, resp.Errors)
		extensions := resp.Errors[0].(map[string]interface{})["extensions"].(map[string]interface{})
		assert.Equal(t, errors.CodeCEError, extensions["code"])
	})

	t.Run("rejects the query when no schema is available", func(t *testing.T) {
		pdp := &fakePolicy{resp: &policy.PdpResponse{AppAuthorized: true}}
		f, err := New(Deps{
			Providers: fakeRegistry{},
			Schemas:   failingSchemaSource{},
			Policy:    pdp,
		}, opts)
		require.NoError(t, err)

		resp := f.FederateQuery(context.Background(), req, assertion)

		assert.Nil(t, pdp.got, "the query should not reach the PDP without a schema")
		require.NotEmpty(t, resp.Errors)
		assert.Contains(t, resp.Errors[0].(*graphql.JSONError).Message, "No active schema found")
	})

	t.Run("sends sub-queries through the injected provider client and audits them", func(t *testing.T) {
		identityRegistry := &fakeProvider{resp: graphql.Response{Data: map[string]interface{}{
			"person": map[string]interface{}{"fullName": "Jane Doe"},
		}}}
		auditor := &recordingAuditor{}
		f, err := New(Deps{
			Providers: fakeRegistry{"identity-registry-schema": identityRegistry},
			Schemas:   staticSchema(schemaSDL),
			Auditor:   auditor,
		}, opts)
		require.NoError(t, err)

		resp := f.FederateQuery(context.Background(), req, assertion)

		require.Empty(t, resp.Errors)
		assert.Equal(t, map[string]interface{}{"fullName": "Jane Doe"}, resp.Data["personInfo"])
		require.Len(t, identityRegistry.got, 1)
		assert.Contains(t, identityRegistry.got[0].Query, `person(nic: $nic)`)
		assert.Equal(t, map[string]interface{}{"nic": "123"}, identityRegistry.got[0].Variables)
		require.Len(t, auditor.fetches, 1)
		assert.Equal(t, ProviderFetch{
			ApplicationID:   "app-123",
			SchemaID:        "identity-registry-schema",
			Query:           identityRegistry.got[0].Query,
			RequestedFields: []string{"person.fullName"},
		}, auditor.fetches[0])
		assert.NoError(t, auditor.errs[0])
	})

	t.Run("audits a failed provider fetch and reports its fields as null", func(t *testing.T) {
		auditor := &recordingAuditor{}
		f, err := New(Deps{
			Providers: fakeRegistry{"identity-registry-schema": &fakeProvider{err: fmt.Errorf("connection refused")}},
			Schemas:   staticSchema(schemaSDL),
			Auditor:   auditor,
		}, opts)
		require.NoError(t, err)

		resp := f.FederateQuery(context.Background(), req, assertion)

		assert.Equal(t, map[string]interface{}{"fullName": nil}, resp.Data["personInfo"])
		assert.Equal(t, []interface{}{map[string]interface{}{
			"message":    "Failed to fetch data from provider for schema identity-registry-schema",
			"path":       []any{"personInfo", "fullName"},
			"extensions": map[string]interface{}{"code": errors.CodeProviderError},
		}}, resp.Errors)
		require.Len(t, auditor.errs, 1)
		assert.EqualError(t, auditor.errs[0], "connection refused")
	})
}

func TestFederateQuery_ValidatesAgainstSchema(t *testing.T) {
	schemaSDL := `
		type Query {
			personInfo(nic: String!): PersonInfo
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerField: "person.fullName", schemaId: "identity-registry-schema")
		}
	`
	opts := Options{
		OwnerIDArgName: "nic",
		ArgMapping: []*graphql.ArgMapping{
			{SchemaID: "identity-registry-schema", TargetArgName: "nic", SourceArgPath: "personInfo-nic", TargetArgPath: "person"},
		},
	}

	tests := []struct {
		name    string
		query   string
		message string
	}{
		{"unknown field", `query { personInfo(nic: "123") { age } }`, `Cannot query field "age" on type "PersonInfo".`},
		{"wrong argument type", `query { personInfo(nic: 123) { fullName } }`, `String cannot represent a non string value: 123`},
		{"missing required argument", `query { personInfo { fullName } }`, `Field "personInfo" argument "nic" of type "String!" is required, but it was not provided.`},
		{"syntax error", `query { personInfo(nic: "123" { fullName } }`, `Expected Name, found {`},
	}
	for _, tt := range tests {
		t.Run("rejects a query with a "+tt.name, func(t *testing.T) {
			pdp := &fakePolicy{resp: &policy.PdpResponse{AppAuthorized: true}}
			identityRegistry := &fakeProvider{}
			f, err := New(Deps{
				Providers: fakeRegistry{"identity-registry-schema": identityRegistry},
				Schemas:   staticSchema(schemaSDL),
				Policy:    pdp,
			}, opts)
			require.NoError(t, err)

			resp := f.FederateQuery(context.Background(), graphql.Request{Query: tt.query}, &auth.ConsumerAssertion{ApplicationID: "app-123"})

			require.Len(t, resp.Errors, 1)
			reported := resp.Errors[0].(map[string]interface{})
			assert.Equal(t, tt.message, reported["message"])
			assert.Equal(t, map[string]interface{}{"code": errors.CodeBadRequest}, reported["extensions"])
			assert.NotEmpty(t, reported["locations"])
			assert.Nil(t, pdp.got, "an invalid query must not reach the PDP")
			assert.Empty(t, identityRegistry.got, "an invalid query must not reach a provider")
		})
	}
}

// TestFederateQuery_SharedSchemaConcurrently runs queries in parallel against one
// cached compiled schema; run with -race to check the schema is only read.
func TestFederateQuery_SharedSchemaConcurrently(t *testing.T) {
	compiled, err := schema.Compile(`
		type Query {
			personInfo(nic: String!): PersonInfo
		}
		type PersonInfo {
			fullName: String @sourceInfo(providerField: "person.fullName", schemaId: "identity-registry-schema")
			address: String @sourceInfo(providerField: "person.address", schemaId: "identity-registry-schema")
		}
	`)
	require.NoError(t, err)
	identityRegistry := &fakeProvider{resp: graphql.Response{Data: map[string]interface{}{
		"person": map[string]interface{}{"fullName": "Jane Doe", "address": "Central City"},
	}}}
	f, err := New(Deps{
		Providers: fakeRegistry{"identity-registry-schema": identityRegistry},
		Schemas:   fixedSchema{compiled},
	}, Options{
		OwnerIDArgName: "nic",
		ArgMapping: []*graphql.ArgMapping{
			{SchemaID: "identity-registry-schema", TargetArgName: "nic", SourceArgPath: "personInfo-nic", TargetArgPath: "person"},
		},
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := f.FederateQuery(context.Background(),
				graphql.Request{Query: `query { personInfo(nic: "123") { fullName address } }`},
				&auth.ConsumerAssertion{ApplicationID: "app-123"})
			assert.Empty(t, resp.Errors)
			assert.Equal(t, map[string]interface{}{"fullName": "Jane Doe", "address": "Central City"}, resp.Data["personInfo"])
		}()
	}
	wg.Wait()
}

// fixedSchema is a SchemaSource that serves one compiled schema.
type fixedSchema struct{ compiled *schema.CompiledSchema }

func (s fixedSchema) Active(context.Context) (*schema.CompiledSchema, error) { return s.compiled, nil }

func TestProviderLookup(t *testing.T) {
	p := &fakeProvider{}
	registry := ProviderLookup(func(schemaID string) (*fakeProvider, bool) {
		if schemaID == "identity-registry-schema" {
			return p, true
		}
		return nil, false
	})

	got, ok := registry.Get("identity-registry-schema")
	require.True(t, ok)
	assert.Same(t, p, got)

	got, ok = registry.Get("unknown")
	assert.False(t, ok)
	assert.Nil(t, got, "a missing provider must be an untyped nil ProviderClient")
}
