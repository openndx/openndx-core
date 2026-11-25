// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package federator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

// fetchResult is the outcome of one fetch: the provider's response, or the error
// that stopped it.
type fetchResult struct {
	response graphql.Response
	err      error
}

// execute runs fetches concurrently and returns their results when all have
// finished. Each provider call is audited.
func (f *Federator) execute(ctx context.Context, appID string, fetches []*fetch) map[*fetch]fetchResult {
	results := make(map[*fetch]fetchResult, len(fetches))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, ft := range fetches {
		client, ok := f.deps.Providers.Get(ft.schemaID)
		if !ok {
			slog.InfoContext(ctx, "Provider not found", "Schema ID", ft.schemaID)
			results[ft] = fetchResult{err: fmt.Errorf("no provider is configured for schema %s", ft.schemaID)}
			continue
		}

		wg.Add(1)
		go func(ft *fetch, client ProviderClient) {
			defer wg.Done()

			audit := ProviderFetch{
				ApplicationID:   appID,
				SchemaID:        ft.schemaID,
				Query:           ft.query,
				RequestedFields: ft.fields,
			}
			response, err := client.Execute(ctx, graphql.Request{Query: ft.query, Variables: ft.variables})
			if err != nil {
				slog.InfoContext(ctx, "Request failed to the Provider", "Schema ID", ft.schemaID, "Error", err)
				f.deps.Auditor.ProviderFetch(ctx, audit, nil, err)
			} else {
				f.deps.Auditor.ProviderFetch(ctx, audit, &response, nil)
			}

			mu.Lock()
			results[ft] = fetchResult{response: response, err: err}
			mu.Unlock()
		}(ft, client)
	}

	wg.Wait()
	return results
}
