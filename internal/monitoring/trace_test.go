package monitoring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsValidTraceID(t *testing.T) {
	t.Parallel()

	validUUID := "123e4567-e89b-12d3-a456-426614174000"
	tooLong := strings.Repeat("a", maxTraceIDLen+1)

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "empty", id: "", want: false},
		{name: "uuid", id: validUUID, want: true},
		{name: "alphanumeric", id: "TraceID123", want: true},
		{name: "too long", id: tooLong, want: false},
		{name: "spaces", id: "bad id", want: false},
		{name: "control chars", id: "bad\nid", want: false},
		{name: "special chars", id: "trace;DROP", want: false},
		{name: "max length", id: strings.Repeat("a", maxTraceIDLen), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isValidTraceID(tt.id); got != tt.want {
				t.Fatalf("isValidTraceID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestExtractTraceIDFromRequest_AcceptsValidHeader(t *testing.T) {
	t.Parallel()

	const want = "abc-123"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, want)

	ctx := ExtractTraceIDFromRequest(req)
	if got := GetTraceIDFromContext(ctx); got != want {
		t.Fatalf("GetTraceIDFromContext() = %q, want %q", got, want)
	}
}

func TestExtractTraceIDFromRequest_RejectsInvalidHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, "bad id\nwith\tjunk")

	ctx := ExtractTraceIDFromRequest(req)
	got := GetTraceIDFromContext(ctx)
	if got == "" {
		t.Fatal("expected a generated trace ID, got empty")
	}
	if got == "bad id\nwith\tjunk" {
		t.Fatal("invalid client trace ID was propagated")
	}
	if !isValidTraceID(got) {
		t.Fatalf("generated trace ID is not valid: %q", got)
	}
}

func TestTraceIDMiddleware_RejectsInvalidHeader(t *testing.T) {
	t.Parallel()

	var seen string
	handler := TraceIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = GetTraceIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, strings.Repeat("x", maxTraceIDLen+1))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if seen == "" {
		t.Fatal("expected a generated trace ID in request context")
	}
	if !isValidTraceID(seen) {
		t.Fatalf("context trace ID is not valid: %q", seen)
	}
	if got := rec.Header().Get(TraceIDHeader); got != seen {
		t.Fatalf("response header = %q, want %q", got, seen)
	}
}

func TestTraceIDMiddleware_PropagatesValidHeader(t *testing.T) {
	t.Parallel()

	const want = "request-trace-42"
	var seen string
	handler := TraceIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = GetTraceIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, want)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if seen != want {
		t.Fatalf("context trace ID = %q, want %q", seen, want)
	}
	if got := rec.Header().Get(TraceIDHeader); got != want {
		t.Fatalf("response header = %q, want %q", got, want)
	}
}
