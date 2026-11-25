// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package monitoring

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// TraceIDHeader is the HTTP header name for trace ID
const TraceIDHeader = "X-Trace-ID"

// traceIDKey is the context key for trace ID
// This is used for distributed tracing and observability correlation
type traceIDKey struct{}

// GetTraceIDFromContext retrieves the trace ID from the context
// Returns empty string if trace ID is not found in context
// This is used for distributed tracing and observability correlation across service boundaries
func GetTraceIDFromContext(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceIDKey{}).(string); ok {
		return traceID
	}
	return ""
}

// WithTraceID adds the given trace ID to the context
// This is used to propagate trace IDs for distributed tracing and observability correlation
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// ExtractTraceIDFromRequest extracts trace ID from HTTP header and adds it to context
// If no trace ID is found in header, generates a new one
// This ensures trace ID propagation across HTTP service boundaries
func ExtractTraceIDFromRequest(r *http.Request) context.Context {
	traceID := r.Header.Get(TraceIDHeader)
	if traceID == "" {
		traceID = uuid.New().String()
	}
	return WithTraceID(r.Context(), traceID)
}

// TraceIDMiddleware extracts or generates a trace ID and adds it to the request context
// It checks for X-Trace-ID header first, and if not present, generates a new UUID
// The trace ID is also set in the response header for client visibility
// This middleware should be applied early in the middleware chain to ensure trace ID
// is available throughout the request lifecycle
func TraceIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for existing trace ID in header
		traceID := r.Header.Get(TraceIDHeader)
		if traceID == "" {
			// Generate new trace ID if not present
			traceID = uuid.New().String()
		}

		// Add trace ID to context using the shared traceIDKey
		ctx := WithTraceID(r.Context(), traceID)

		// Set trace ID in response header for client visibility
		w.Header().Set(TraceIDHeader, traceID)

		// Continue with the updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
