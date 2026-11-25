// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	"context"
	"testing"

	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	OwnerIDArgName string
	ArgMapping     []*graphql.ArgMapping
	PdpURL         string
	CeURL          string
	Providers      []provider.Config
}

// newTestFederator builds a Federator from cfg and schemas with the real PDP,
// consent and provider clients, the way cmd/oe/main.go does.
func newTestFederator(t *testing.T, cfg *testConfig, schemas SchemaSource) *Federator {
	t.Helper()

	registry, err := provider.NewRegistry(cfg.Providers...)
	require.NoError(t, err)

	deps := Deps{
		Providers: ProviderLookup(registry.Get),
		Schemas:   schemas,
	}
	if cfg.PdpURL != "" {
		deps.Policy = policy.NewPdpClient(cfg.PdpURL)
	}
	if cfg.CeURL != "" {
		deps.Consent = consent.NewCEServiceClient(cfg.CeURL)
	}
	f, err := New(deps, Options{
		OwnerIDArgName: cfg.OwnerIDArgName,
		ArgMapping:     cfg.ArgMapping,
	})
	require.NoError(t, err)
	return f
}

// staticSchema is a SchemaSource that always serves the same SDL.
type staticSchema string

func (s staticSchema) Active(context.Context) (*schema.CompiledSchema, error) {
	return schema.Compile(string(s))
}
