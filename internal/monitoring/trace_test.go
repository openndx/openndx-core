package monitoring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidTraceID(t *testing.T) {
	t.Parallel()

	validUUID := uuid.New().String()
	tests := []struct {
		name  string
		id    string
		valid bool
	}{
		{name: "empty", id: "", valid: false},
		{name: "uuid", id: validUUID, valid: true},
		{name: "alphanumeric hyphen", id: "abc-123-XYZ", valid: true},
		{name: "max length", id: strings.Repeat("a", maxTraceIDLen), valid: true},
		{name: "too long", id: strings.Repeat("a", maxTraceIDLen+1), valid: false},
		{name: "space", id: "bad id", valid: false},
		{name: "newline", id: "trace\nid", valid: false},
		{name: "control char", id: "trace\x00id", valid: false},
		{name: "slash", id: "a/b", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.valid, isValidTraceID(tt.id))
		})
	}
}

func TestTraceIDMiddleware_RejectsInvalidHeader(t *testing.T) {
	t.Parallel()

	handler := TraceIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, GetTraceIDFromContext(r.Context()), w.Header().Get(TraceIDHeader))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, "evil\ninjected")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	got := rec.Header().Get(TraceIDHeader)
	require.NotEmpty(t, got)
	assert.NotEqual(t, "evil\ninjected", got)
	assert.True(t, isValidTraceID(got))
}

func TestTraceIDMiddleware_AcceptsValidHeader(t *testing.T) {
	t.Parallel()

	want := uuid.New().String()
	handler := TraceIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, want, GetTraceIDFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, want)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, want, rec.Header().Get(TraceIDHeader))
}

func TestExtractTraceIDFromRequest_RejectsInvalidHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, strings.Repeat("x", maxTraceIDLen+1))

	ctx := ExtractTraceIDFromRequest(req)
	got := GetTraceIDFromContext(ctx)

	require.NotEmpty(t, got)
	assert.NotEqual(t, req.Header.Get(TraceIDHeader), got)
	assert.True(t, isValidTraceID(got))
}
