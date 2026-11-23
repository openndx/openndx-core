package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondWithServiceError(t *testing.T) {
	errNotFound := errors.New("not found")
	errInvalid := errors.New("invalid")
	statuses := []ErrorStatus{
		{Err: errNotFound, Status: http.StatusNotFound},
		{Err: errInvalid, Status: http.StatusBadRequest},
	}

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"matched", errNotFound, http.StatusNotFound, "not found"},
		{"wrapped", fmt.Errorf("field x: %w", errInvalid), http.StatusBadRequest, "field x: invalid"},
		{"unmatched", errors.New("connection refused"), http.StatusInternalServerError, "internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			RespondWithServiceError(w, tt.err, statuses...)

			assert.Equal(t, tt.wantStatus, w.Code)
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tt.wantBody, body.Error)
		})
	}
}

func TestReadinessHandler(t *testing.T) {
	t.Run("a reachable dependency reports ready", func(t *testing.T) {
		w := httptest.NewRecorder()
		ReadinessHandler(func(context.Context) error { return nil }).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/readiness", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
	})

	t.Run("an unreachable dependency reports unavailable", func(t *testing.T) {
		w := httptest.NewRecorder()
		ReadinessHandler(func(context.Context) error { return errors.New("dial tcp db:5432: refused") }).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/readiness", nil))

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.JSONEq(t, `{"status":"unavailable"}`, w.Body.String())
		assert.NotContains(t, w.Body.String(), "db:5432", "the response must not carry internal details")
	})

	t.Run("a service without a dependency reports ready", func(t *testing.T) {
		w := httptest.NewRecorder()
		ReadinessHandler(nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/readiness", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
	})

	t.Run("the dependency check gets a deadline", func(t *testing.T) {
		var deadline time.Time
		var hasDeadline bool
		w := httptest.NewRecorder()
		ReadinessHandler(func(ctx context.Context) error {
			deadline, hasDeadline = ctx.Deadline()
			return nil
		}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/readiness", nil))

		assert.Equal(t, http.StatusOK, w.Code)
		require.True(t, hasDeadline, "the ping must run under a deadline")
		assert.WithinDuration(t, time.Now().Add(ReadinessTimeout), deadline, time.Second)
	})
}
