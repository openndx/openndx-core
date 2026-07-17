package utils

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/OpenNDX/openndx-core/exchange/consent-engine/internal/models"
)

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// RespondWithJSON sends a JSON response with the given status code
func RespondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// If encoding fails, log it but don't try to send another response
		// as headers have already been written
		slog.Error("Failed to encode JSON response", "error", err, "statusCode", statusCode)
		return
	}
}

// RespondWithError sends a JSON error response with the given status code
// This version accepts models.ConsentErrorCode for type-safe error codes
func RespondWithError(w http.ResponseWriter, statusCode int, errorCode models.ConsentErrorCode, message string) {
	response := ErrorResponse{}
	response.Error.Code = string(errorCode)
	response.Error.Message = message

	RespondWithJSON(w, statusCode, response)
}
