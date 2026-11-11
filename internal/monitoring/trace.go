package monitoring

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// TraceIDHeader is the HTTP header name for trace ID
const TraceIDHeader = "X-Trace-ID"

// maxTraceIDLen is the maximum accepted length for a client-supplied trace ID.
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
// Empty, oversized, or non alphanumeric/hyphen values are rejected so they cannot
// pollute logs or break downstream parsers.
func isValidTraceID(id string) bool {
	if id == "" || len(id) > maxTraceIDLen {
		return false
	}
	for _, c := range id {
		if !(c == '-' ||
			(c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// resolveTraceID returns the request header value when valid, otherwise a new UUID.
func resolveTraceID(r *http.Request) string {
	traceID := r.Header.Get(TraceIDHeader)
	if !isValidTraceID(traceID) {
		return uuid.New().String()
	}
	return traceID
}

// ExtractTraceIDFromRequest extracts trace ID from HTTP header and adds it to context.
// If no valid trace ID is found in the header, generates a new one.
// This ensures trace ID propagation across HTTP service boundaries.
func ExtractTraceIDFromRequest(r *http.Request) context.Context {
	return WithTraceID(r.Context(), resolveTraceID(r))
}

// TraceIDMiddleware extracts or generates a trace ID and adds it to the request context.
// It accepts X-Trace-ID only when the value has a bounded, safe shape; otherwise it
// generates a new UUID. The trace ID is also set in the response header for client visibility.
// This middleware should be applied early in the middleware chain to ensure trace ID
// is available throughout the request lifecycle.
func TraceIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := resolveTraceID(r)
		ctx := WithTraceID(r.Context(), traceID)
		w.Header().Set(TraceIDHeader, traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
