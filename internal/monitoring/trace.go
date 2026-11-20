package monitoring

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// TraceIDHeader is the HTTP header name for trace ID
const TraceIDHeader = "X-Trace-ID"

// maxTraceIDLen bounds client-supplied trace IDs to limit log pollution and injection risk.
const maxTraceIDLen = 64

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

// isValidTraceID reports whether a client-supplied trace ID is safe to propagate.
func isValidTraceID(id string) bool {
	if id == "" || len(id) > maxTraceIDLen {
		return false
	}
	for _, c := range id {
		if !(c == '-' || (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// resolveTraceID returns the client-supplied ID when valid, otherwise a new UUID.
func resolveTraceID(headerValue string) string {
	if isValidTraceID(headerValue) {
		return headerValue
	}
	return uuid.New().String()
}

// ExtractTraceIDFromRequest extracts trace ID from HTTP header and adds it to context
// If no valid trace ID is found in header, generates a new one
// This ensures trace ID propagation across HTTP service boundaries
func ExtractTraceIDFromRequest(r *http.Request) context.Context {
	traceID := resolveTraceID(r.Header.Get(TraceIDHeader))
	return WithTraceID(r.Context(), traceID)
}

// TraceIDMiddleware extracts or generates a trace ID and adds it to the request context
// It accepts X-Trace-ID only when the value has a bounded alphanumeric/hyphen shape;
// otherwise it generates a new UUID. The trace ID is also set in the response header.
// This middleware should be applied early in the middleware chain to ensure trace ID
// is available throughout the request lifecycle
func TraceIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := resolveTraceID(r.Header.Get(TraceIDHeader))

		// Add trace ID to context using the shared traceIDKey
		ctx := WithTraceID(r.Context(), traceID)

		// Set trace ID in response header for client visibility
		w.Header().Set(TraceIDHeader, traceID)

		// Continue with the updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
