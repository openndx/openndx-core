// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	"context"
	"errors"
	"fmt"

	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
)

// SchemaSource supplies the active consumer schema.
type SchemaSource interface {
	// Active returns the compiled schema consumer queries are validated and federated against.
	Active(ctx context.Context) (*schema.CompiledSchema, error)
}

// PolicyDecider asks the Policy Decision Point whether a consumer may access the requested fields.
type PolicyDecider interface {
	MakePdpRequest(ctx context.Context, request *policy.PdpRequest) (*policy.PdpResponse, error)
}

// ConsentRequester asks the Consent Engine for the data owner's consent.
type ConsentRequester interface {
	CreateConsent(ctx context.Context, request *consent.CreateConsentRequest) (*consent.ConsentResponseInternalView, error)
}

// ProviderRegistry looks up the provider that serves a schema.
type ProviderRegistry interface {
	Get(schemaID string) (ProviderClient, bool)
}

// ProviderClient sends a sub-query to one provider.
type ProviderClient interface {
	Execute(ctx context.Context, request graphql.Request) (graphql.Response, error)
}

// ProviderLookup adapts lookup, which returns a concrete client type, to a ProviderRegistry.
func ProviderLookup[C ProviderClient](lookup func(schemaID string) (C, bool)) ProviderRegistry {
	return providerLookup[C](lookup)
}

type providerLookup[C ProviderClient] func(schemaID string) (C, bool)

func (l providerLookup[C]) Get(schemaID string) (ProviderClient, bool) {
	client, ok := l(schemaID)
	if !ok {
		// Avoid returning a typed nil as a non-nil ProviderClient.
		return nil, false
	}
	return client, true
}

// ProviderFetch describes a sub-query sent to a provider, for auditing.
type ProviderFetch struct {
	ApplicationID string
	SchemaID      string
	Query         string
	// RequestedFields are the provider field paths the consumer asked this provider for.
	RequestedFields []string
}

// Auditor records the audit events emitted while federating a query. The methods
// that return a context return ctx with a trace ID added if it had none.
type Auditor interface {
	RequestReceived(ctx context.Context, appID, query string) context.Context
	PolicyCheck(ctx context.Context, appID string, req *policy.PdpRequest, resp *policy.PdpResponse, err error) context.Context
	ConsentCheck(ctx context.Context, appID, ownerID string, req *consent.CreateConsentRequest, resp *consent.ConsentResponseInternalView, err error) context.Context
	ProviderFetch(ctx context.Context, fetch ProviderFetch, resp *graphql.Response, err error)
}

// Deps are the collaborators the Federator calls out to. Providers and Schemas are required.
type Deps struct {
	Providers ProviderRegistry
	Schemas   SchemaSource
	// Policy is skipped when nil.
	Policy PolicyDecider
	// Consent is required only when the PDP asks for owner consent.
	Consent ConsentRequester
	// Auditor defaults to one that records nothing.
	Auditor Auditor
}

// Options configure how queries are federated.
type Options struct {
	// OwnerIDArgName identifies the Data Owner ID argument by name in every
	// federated query. It is read only to ask the owner's consent, so it is
	// required when Deps.Consent is set.
	OwnerIDArgName string
	ArgMapping     []*graphql.ArgMapping
}

// New returns a Federator that uses deps and opts.
// It fails fast on configuration that would reject every request.
func New(deps Deps, opts Options) (*Federator, error) {
	if deps.Providers == nil {
		return nil, errors.New("fatal configuration error: a provider registry is required")
	}
	if deps.Schemas == nil {
		return nil, errors.New("fatal configuration error: a schema source is required")
	}
	// Without OwnerIDArgName every query that needs consent would be rejected.
	if deps.Consent != nil && opts.OwnerIDArgName == "" {
		return nil, fmt.Errorf("fatal configuration error: ownerIdArgName is not configured - cannot identify the Data Owner ID argument for consent")
	}
	targetTypes, err := targetArgTypes(opts.ArgMapping)
	if err != nil {
		return nil, fmt.Errorf("fatal configuration error: %w", err)
	}
	if deps.Auditor == nil {
		deps.Auditor = noopAuditor{}
	}
	return &Federator{deps: deps, opts: opts, targetTypes: targetTypes}, nil
}

type noopAuditor struct{}

func (noopAuditor) RequestReceived(ctx context.Context, _, _ string) context.Context { return ctx }

func (noopAuditor) PolicyCheck(ctx context.Context, _ string, _ *policy.PdpRequest, _ *policy.PdpResponse, _ error) context.Context {
	return ctx
}

func (noopAuditor) ConsentCheck(ctx context.Context, _, _ string, _ *consent.CreateConsentRequest, _ *consent.ConsentResponseInternalView, _ error) context.Context {
	return ctx
}

func (noopAuditor) ProviderFetch(context.Context, ProviderFetch, *graphql.Response, error) {}
