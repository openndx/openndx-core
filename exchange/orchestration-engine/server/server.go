package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/auth"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/database"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/federator"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/handlers"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/pkg/graphql"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/services"
	"github.com/go-chi/chi/v5"
)

type Response struct {
	Message string `json:"message"`
}

const DefaultPort = "4000"

// RunServer starts an HTTP server with graceful shutdown support.
// The server will shut down gracefully when the context is cancelled (e.g., on SIGINT/SIGTERM).
func RunServer(ctx context.Context, f *federator.Federator) {
	mux := SetupRouter(f)

	// Get port configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = DefaultPort
	}

	// Convert port to string with colon prefix
	if port[0] != ':' {
		port = ":" + port
	}

	// Create HTTP server with proper configuration
	srv := &http.Server{
		Addr:    port,
		Handler: corsMiddleware(mux),
	}

	// Channel to signal server errors
	serverErrors := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		logger.Log.Info("Server is Listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		logger.Log.Error("Server error", "error", err)
	case <-ctx.Done():
		logger.Log.Info("Shutdown signal received, starting graceful shutdown")

		// Create shutdown context with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Log.Error("Server shutdown error", "error", err)
			// Force close if graceful shutdown fails
			if closeErr := srv.Close(); closeErr != nil {
				logger.Log.Error("Server close error", "error", closeErr)
			}
		} else {
			logger.Log.Info("Server stopped gracefully")
		}
	}
}

// SetupRouter initializes the router and registers all endpoints
func SetupRouter(f *federator.Federator) *chi.Mux {
	mux := chi.NewRouter()

	// Initialize database connection
	dbConnectionString := getDatabaseConnectionString()
	schemaDB, err := database.NewSchemaDB(dbConnectionString)
	if err != nil {
		logger.Log.Error("Failed to connect to database", "error", err)
		// Continue without database for now
		schemaDB = nil
	}

	// Initialize schema service and handler
	var schemaService handlers.SchemaService
	if schemaDB != nil {
		schemaService = services.NewSchemaService(schemaDB)
	} else {
		// Fallback to in-memory service if database is not available
		schemaService = nil
		logger.Log.Warn("Running without database - schema management disabled")
	}

	schemaHandler := handlers.NewSchemaHandler(schemaService)

	// Set the schema service in the federator
	f.SchemaService = schemaService
	// /health route
	mux.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp := Response{Message: "OpenDIF Server is Healthy!"}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(resp)
		if err != nil {
			return
		}
	})

	// Schema management routes
	mux.Get("/sdl", schemaHandler.GetActiveSchema)
	mux.Post("/sdl", schemaHandler.CreateSchema)
	mux.Get("/sdl/versions", schemaHandler.GetSchemas)
	mux.Post("/sdl/validate", schemaHandler.ValidateSDL)
	mux.Post("/sdl/check-compatibility", schemaHandler.CheckCompatibility)

	// Handle activation endpoint with proper path matching
	mux.Post("/sdl/versions/{version}/activate", schemaHandler.ActivateSchema)

	// Publicly accessible Endpoints
	mux.Post("/public/graphql", func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req graphql.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Log.Error("Failed to decode request body", "error", err)
			http.Error(w, "Bad request: invalid JSON", http.StatusBadRequest)
			return
		}

		// decode the token using the cached TokenValidator
		consumerAssertion, err := auth.GetConsumerJwtFromTokenWithValidator(f.Configs.Environment, &f.Configs.JWT, f.Configs.TrustUpstream, r, f.TokenValidator)
		if err != nil {
			logger.Log.Error("Failed to get consumer JWT from token", "error", err)
			// Return generic error to client to avoid exposing internal details
			http.Error(w, "Unauthorized: invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Add panic recovery for federator calls
		var response graphql.Response
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Log.Error("Panic in FederateQuery", "panic", r, "stack", string(debug.Stack()))
					response = graphql.Response{
						Data: nil,
						Errors: []interface{}{
							map[string]interface{}{
								"message": fmt.Sprintf("Internal server error: %v", r),
							},
						},
					}
				}
			}()
			response = f.FederateQuery(r.Context(), req, consumerAssertion)
		}()

		w.WriteHeader(http.StatusOK)
		// Set content type to application/json

		w.Header().Set("Content-Type", "application/json")

		err = json.NewEncoder(w).Encode(response)
		if err != nil {
			logger.Log.Error("Failed to write response", "error", err)
			return
		}
	})

	return mux
}

// corsMiddleware sets CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow all origins
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Allow specific methods
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		// Allow specific headers
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight (OPTIONS) requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getDatabaseConnectionString returns the database connection string from environment variables
func getDatabaseConnectionString() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "")
	dbname := getEnv("DB_NAME", "orchestration_engine")
	sslmode := getEnv("DB_SSLMODE", "disable")

	// Require password from environment - no default
	if password == "" {
		logger.Log.Warn("DB_PASSWORD not set - database connection may fail")
	}

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)
}

// getEnv gets an environment variable with a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
