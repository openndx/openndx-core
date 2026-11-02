package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/openndx/openndx-core/internal/database"
	"github.com/openndx/openndx-core/internal/logging"
	"github.com/openndx/openndx-core/internal/pdp/engine"
	"github.com/openndx/openndx-core/internal/pdp/handler"
	"github.com/openndx/openndx-core/internal/utils"
)

// Build information - set during build
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// main reports startup failures after run has released its resources.
func main() {
	if err := run(); err != nil {
		slog.Error("Policy decision point failed", "error", err)
		os.Exit(1)
	}
}

// run loads validated configuration and owns the PDP's resources and server.
func run() error {
	cfg, err := loadConfig(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logging.Setup(cfg.Logging, "policy-decision-point", cfg.Environment, Version)

	slog.Info("Starting policy decision point",
		"port", cfg.Server.Port,
		"build_time", BuildTime,
		"git_commit", GitCommit)

	gormDB, sqlDB, err := database.OpenGORM(context.Background(), cfg.Database.Config, cfg.Database.IdleTimeout)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			slog.Error("Failed to close database connection", "error", err)
		} else {
			slog.Info("Database connection closed successfully")
		}
	}()

	policyEngine, err := engine.New(context.Background())
	if err != nil {
		return fmt.Errorf("initialize policy engine: %w", err)
	}

	// Initialize V1 handlers
	v1Handler := handler.NewHandler(gormDB, policyEngine)

	// Setup routes
	mux := http.NewServeMux()

	// Policy endpoints
	mux.HandleFunc("GET /api/v1/policy/metadata", v1Handler.ListPolicyMetadata)
	mux.HandleFunc("POST /api/v1/policy/metadata", v1Handler.CreatePolicyMetadata)
	mux.HandleFunc("PATCH /api/v1/policy/metadata/{id}", v1Handler.PatchPolicyMetadata)
	mux.HandleFunc("DELETE /api/v1/policy/metadata/{id}", v1Handler.DeletePolicyMetadata)
	mux.HandleFunc(
		"DELETE /api/v1/policy/metadata/{id}/allowlist/{applicationId}",
		v1Handler.RevokeAllowListEntry,
	)
	mux.HandleFunc("POST /api/v1/policy/update-allowlist", v1Handler.UpdateAllowList)
	mux.HandleFunc("POST /api/v1/policy/decide", v1Handler.GetPolicyDecision)

	// Health check endpoint
	mux.Handle("GET /health", utils.HealthHandler("policy-decision-point"))

	// Debug endpoint
	mux.HandleFunc("GET /debug", func(w http.ResponseWriter, r *http.Request) {
		utils.RespondWithJSON(w, http.StatusOK, map[string]string{
			"service": "policy-decision-point",
			"version": Version,
			"path":    r.URL.Path,
			"method":  r.Method,
		})
	})

	// Database debug endpoint
	mux.HandleFunc("GET /debug/db", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		debugInfo := map[string]interface{}{
			"service": "policy-decision-point",
			"v1":      map[string]interface{}{},
		}

		// Test V1 GORM database connection
		if gormDB == nil {
			debugInfo["v1"] = map[string]interface{}{
				"error": "GORM connection is nil",
			}
		} else {
			sqlDB, err := gormDB.DB()
			if err != nil {
				debugInfo["v1"] = map[string]interface{}{
					"error": "failed to get sql.DB: " + err.Error(),
				}
			} else if err := sqlDB.PingContext(ctx); err != nil {
				debugInfo["v1"] = map[string]interface{}{
					"error": "database ping failed: " + err.Error(),
				}
			} else {
				v1Info := map[string]interface{}{
					"status":   "connected",
					"database": cfg.Database.Postgres.Name,
				}

				// Check if policy_metadata table exists
				var tableExists bool
				checkTableQuery := `SELECT EXISTS (
				       SELECT FROM information_schema.tables 
				       WHERE table_schema = 'public' 
				       AND table_name = 'policy_metadata'
			       )`

				if err := sqlDB.QueryRowContext(ctx, checkTableQuery).Scan(&tableExists); err != nil {
					v1Info["table_check_error"] = "failed to check policy_metadata table: " + err.Error()
				} else {
					v1Info["policy_metadata_table_exists"] = tableExists
					if tableExists {
						var count int
						countQuery := `SELECT COUNT(*) FROM policy_metadata`
						if err := sqlDB.QueryRowContext(ctx, countQuery).Scan(&count); err != nil {
							v1Info["count_error"] = "failed to count policy_metadata: " + err.Error()
						} else {
							v1Info["policy_metadata_count"] = count
						}
					}
				}
				debugInfo["v1"] = v1Info
			}
		}

		utils.RespondWithJSON(w, http.StatusOK, debugInfo)
	})

	// Create server configuration
	serverConfig := &utils.ServerConfig{
		Port:         strconv.Itoa(cfg.Server.Port),
		ReadTimeout:  cfg.Server.Timeout,
		WriteTimeout: cfg.Server.Timeout,
		IdleTimeout:  60 * time.Second,
	}
	server := utils.CreateServer(serverConfig, utils.PanicRecoveryMiddleware(mux))

	// Start server with graceful shutdown
	if err := utils.StartServerWithGracefulShutdown(server); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
