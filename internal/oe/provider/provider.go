// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/shared/validation"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

// DefaultTimeout bounds a provider call when Config.Timeout is unset.
const DefaultTimeout = "10s"

// Config configures one data provider: id, url, timeout, auth and tls. The id
// is the provider schema's ID in Portal Backend and the PDP, and matches
// schemaId in the schema's @sourceInfo and in argMappings.
type Config = remote.ServiceConfig

// Registry holds the configured providers keyed by schema ID.
type Registry struct {
	providers map[string]*Provider
}

// NewRegistry validates configs and builds a Registry whose providers call
// their endpoints through OpenNSW/core/remote.
func NewRegistry(configs ...Config) (*Registry, error) {
	providers := make(map[string]*Provider, len(configs))
	for i, cfg := range configs {
		if cfg.ID == "" {
			return nil, fmt.Errorf("providers[%d].id is required", i)
		}
		if err := validation.HTTPURL(fmt.Sprintf("providers[%d].url", i), cfg.URL); err != nil {
			return nil, err
		}
		warnPlainHTTPCredentials(i, cfg)
		if _, exists := providers[cfg.ID]; exists {
			return nil, fmt.Errorf("duplicate provider for schema %q", cfg.ID)
		}

		if cfg.Timeout == "" {
			cfg.Timeout = DefaultTimeout
		}
		if _, err := time.ParseDuration(cfg.Timeout); err != nil {
			return nil, fmt.Errorf("providers[%d] (%s): invalid timeout %q: %w", i, cfg.ID, cfg.Timeout, err)
		}

		mgr := remote.NewManager()
		if err := mgr.SetServices(cfg); err != nil {
			return nil, err
		}
		client, err := mgr.GetClient(cfg.ID)
		if err != nil {
			return nil, err
		}

		providers[cfg.ID] = &Provider{
			schemaID: cfg.ID,
			url:      cfg.URL,
			client:   client,
		}
	}
	return &Registry{providers: providers}, nil
}

// warnPlainHTTPCredentials logs a warning when cfg sends credentials over plain
// HTTP: to its url when it has auth, or to its OAuth2 token URL. HTTP stays
// allowed for local and Docker setups.
func warnPlainHTTPCredentials(i int, cfg Config) {
	if cfg.Auth == nil {
		return
	}
	if strings.HasPrefix(strings.ToLower(cfg.URL), "http://") {
		slog.Warn("Provider credentials are sent over plain HTTP; use https outside local development",
			"provider", cfg.ID, "key", fmt.Sprintf("providers[%d].url", i))
	}
	if cfg.Auth.OAuth2 != nil && strings.HasPrefix(strings.ToLower(cfg.Auth.OAuth2.TokenURL), "http://") {
		slog.Warn("Provider client secret is sent over plain HTTP; use https outside local development",
			"provider", cfg.ID, "key", fmt.Sprintf("providers[%d].auth.oauth2.tokenURL", i))
	}
}

// Get returns the provider registered for schemaID.
func (r *Registry) Get(schemaID string) (*Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.providers[schemaID]
	return p, ok
}

// Provider sends GraphQL sub-queries to one configured provider endpoint.
type Provider struct {
	schemaID string
	url      string
	client   *remote.Client
}

// Execute sends request to the provider and decodes its GraphQL response. A
// response with a non-2xx HTTP status is an error, whatever its body.
func (p *Provider) Execute(ctx context.Context, request graphql.Request) (graphql.Response, error) {
	resp, err := p.client.RawRequest(ctx, remote.Request{
		Method: http.MethodPost,
		Path:   p.url,
		Body:   remote.JSONBody{V: request},
	})
	if err != nil {
		return graphql.Response{}, err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return graphql.Response{}, fmt.Errorf("provider for schema %s returned HTTP status %d", p.schemaID, resp.StatusCode)
	}

	var result graphql.Response
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return graphql.Response{}, err
	}
	return result, nil
}
