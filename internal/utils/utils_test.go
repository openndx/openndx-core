package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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
