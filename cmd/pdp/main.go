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
	"github.com/openndx/openndx-core/internal/pdp/decision"
	"github.com/openndx/openndx-core/internal/pdp/fieldpolicy"
	"github.com/openndx/openndx-core/internal/pdp/grant"
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

	// Initialize services and handlers
	fieldPolicyService := fieldpolicy.NewService(gormDB)
	fieldPolicyHandler := fieldpolicy.NewHandler(fieldPolicyService)
	grantService := grant.NewService(gormDB)
	grantHandler := grant.NewHandler(grantService)
	decisionHandler := decision.NewHandler(decision.NewService(fieldPolicyService, grantService))

	// Setup routes
	mux := http.NewServeMux()

	// Policy endpoints
	mux.HandleFunc("GET /api/v1/policy/metadata", fieldPolicyHandler.ListPolicyMetadata)
	mux.HandleFunc("POST /api/v1/policy/metadata", fieldPolicyHandler.CreatePolicyMetadata)
	mux.HandleFunc("PATCH /api/v1/policy/metadata/{id}", fieldPolicyHandler.PatchPolicyMetadata)
	mux.HandleFunc("DELETE /api/v1/policy/metadata/{id}", fieldPolicyHandler.DeletePolicyMetadata)
	mux.HandleFunc("GET /api/v1/policy/grants", grantHandler.ListAccessGrants)
	mux.HandleFunc("POST /api/v1/policy/grants", grantHandler.CreateAccessGrants)
	mux.HandleFunc("GET /api/v1/policy/grants/{id}", grantHandler.GetAccessGrant)
	mux.HandleFunc("POST /api/v1/policy/grants/{id}/revoke", grantHandler.RevokeAccessGrant)
	mux.HandleFunc("POST /api/v1/policy/decide", decisionHandler.GetPolicyDecision)

	// Health check endpoint
	mux.Handle("GET /health", utils.HealthHandler("policy-decision-point"))

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
