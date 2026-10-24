package federator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/openndx/openndx-core/internal/oe/auth"
	"github.com/openndx/openndx-core/internal/oe/logger"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
)

// QueryFederator federates consumer GraphQL queries across providers.
type QueryFederator interface {
	FederateQuery(ctx context.Context, request graphql.Request, consumerInfo *auth.ConsumerAssertion) graphql.Response
}

// Handler serves POST /public/graphql: it federates the request body's query
// for the consumer that authentication added to the request context, and
// answers with the GraphQL response and HTTP 200. Register it behind
// internal/authn's RequireAuthMiddleware.
func Handler(f QueryFederator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		consumerAssertion := auth.ConsumerFromContext(r.Context())
		if consumerAssertion == nil {
			// Authentication always sets the consumer; fail closed if it did not.
			logger.Log.Error("Consumer identity missing from an authenticated request")
			http.Error(w, "Unauthorized: authentication required", http.StatusUnauthorized)
			return
		}

		var req graphql.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Log.Error("Failed to decode request body", "error", err)
			http.Error(w, "Bad request: invalid JSON", http.StatusBadRequest)
			return
		}

		var response graphql.Response
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Log.Error("Panic in FederateQuery", "panic", r, "stack", string(debug.Stack()))
					response = graphql.Response{
						Errors: []any{
							map[string]any{
								"message": fmt.Sprintf("Internal server error: %v", r),
							},
						},
					}
				}
			}()
			response = f.FederateQuery(r.Context(), req, consumerAssertion)
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Log.Error("Failed to write response", "error", err)
		}
	})
}
