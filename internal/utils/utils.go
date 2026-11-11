package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// ErrorResponse represents a standard error response structure
type ErrorResponse struct {
	Error string `json:"error"`
}

// RespondWithJSON sends a JSON response with the given status code and data
func RespondWithJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON response", "error", err)
	}
}

// RespondWithError sends a JSON error response
func RespondWithError(w http.ResponseWriter, statusCode int, message string) {
	RespondWithJSON(w, statusCode, ErrorResponse{Error: message})
}

// ErrorStatus maps a sentinel error to the HTTP status a handler responds with
// when a service returns it.
type ErrorStatus struct {
	Err    error
	Status int
}

// RespondWithServiceError writes err as a JSON error response. The first entry
// in statuses whose Err matches err (by errors.Is) sets the status, and the
// error message is returned to the caller. An unmatched error is logged and
// answered with 500 and a generic message, so internal details don't leak.
func RespondWithServiceError(w http.ResponseWriter, err error, statuses ...ErrorStatus) {
	for _, s := range statuses {
		if errors.Is(err, s.Err) {
			RespondWithError(w, s.Status, err.Error())
			return
		}
	}

	slog.Error("Service request failed", "error", err)
	RespondWithError(w, http.StatusInternalServerError, "internal server error")
}

// RespondWithSuccess sends a JSON success response
func RespondWithSuccess(w http.ResponseWriter, statusCode int, data interface{}) {
	RespondWithJSON(w, statusCode, data)
}

// HealthHandler creates a health check handler
func HealthHandler(serviceName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{
			"service": serviceName,
			"status":  "healthy",
		}
		RespondWithJSON(w, http.StatusOK, response)
	}
}

// PanicRecoveryMiddleware provides panic recovery for HTTP handlers
func PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.ErrorContext(r.Context(), "Handler panicked", "error", err, "path", r.URL.Path)
				RespondWithError(w, http.StatusInternalServerError, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ServerConfig holds configuration for HTTP servers
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// StartServerWithGracefulShutdown starts an HTTP server and blocks until it
// stops. On SIGINT or SIGTERM it waits up to 30 seconds for active requests to
// finish before returning. Log lines name the service through the logger
// internal/logging set up.
func StartServerWithGracefulShutdown(server *http.Server) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Server starting", "address", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		// ListenAndServe returned before any signal, so this is always an error.
		slog.Error("Server failed to start", "error", err)
		return err
	case <-ctx.Done():
	}

	slog.Info("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown returns once active requests finish or shutdownCtx expires.
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown error", "error", err)
		return err
	}
	return nil
}

// CreateServer creates an HTTP server with the given configuration
func CreateServer(config *ServerConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf(":%s", config.Port),
		Handler:      handler,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}
}
