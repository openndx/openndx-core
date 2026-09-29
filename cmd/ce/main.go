package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/openndx/openndx-core/internal/ce/auth"
	"github.com/openndx/openndx-core/internal/ce/config"
	"github.com/openndx/openndx-core/internal/ce/handlers"
	"github.com/openndx/openndx-core/internal/ce/router"
	"github.com/openndx/openndx-core/internal/ce/services"
	"github.com/openndx/openndx-core/internal/database"
	"github.com/openndx/openndx-core/internal/monitoring"
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
		slog.Error("Consent engine failed", "error", err)
		os.Exit(1)
	}
}

// run loads validated configuration and owns the consent engine's resources and server.
func run() error {
	// Load configuration using flags
	cfg, err := config.LoadConfig("consent-engine")
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Setup logging
	utils.SetupLogging(cfg.Logging.Format, cfg.Logging.Level)

	slog.Info("Starting consent engine",
		"environment", cfg.Environment,
		"port", cfg.Service.Port,
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit)

	// Temporary GORM initialization until #631 switches to database.Open.
	gormDB, sqlDB, err := database.OpenGORM(context.Background(), cfg.Database, cfg.DatabaseIdleTimeout)
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

	// Get consent portal URL from environment
	slog.Info("Using consent portal URL", "url", cfg.ConsentPortalUrl)

	// Initialize consent service
	consentService, err := services.NewConsentService(gormDB, cfg.ConsentPortalUrl)
	if err != nil {
		return fmt.Errorf("initialize consent service: %w", err)
	}

	// Initialize handlers
	internalHandler := handlers.NewInternalHandler(consentService)
	portalHandler := handlers.NewPortalHandler(consentService)

	slog.Info("JWT verifier configuration",
		"client_id", cfg.IDPConfig.ClientID,
		"issuer", cfg.IDPConfig.Issuer,
		"audience", cfg.IDPConfig.Audience,
		"jwks_url", cfg.IDPConfig.JwksUrl,
		"subject_claim", cfg.IDPConfig.SubjectClaim,
		"jwks_insecure_skip_verify", cfg.IDPConfig.InsecureSkipVerify)

	jwtVerifier, err := auth.NewJWTVerifier(auth.JWTVerifierConfig{
		JWKSUrl:            cfg.IDPConfig.JwksUrl,
		Issuer:             cfg.IDPConfig.Issuer,
		Audience:           cfg.IDPConfig.Audience,
		ClientID:           cfg.IDPConfig.ClientID,
		SubjectClaim:       cfg.IDPConfig.SubjectClaim,
		InsecureSkipVerify: cfg.IDPConfig.InsecureSkipVerify,
	})
	if err != nil {
		return fmt.Errorf("initialize JWT verifier: %w", err)
	}

	// Initialize router and register all routes
	apiRouter := router.NewRouter(cfg.Service.AllowedOrigins, internalHandler, portalHandler, jwtVerifier)
	mux := http.NewServeMux()

	slog.Info("Registering API routes")
	apiRouter.RegisterRoutes(mux)
	slog.Info("API routes registered successfully")

	// Register legacy /health endpoint for compatibility with health checks
	mux.Handle("/health", utils.PanicRecoveryMiddleware(utils.HealthHandler("consent-engine")))

	// Register /metrics endpoint for Prometheus scraping
	mux.Handle("/metrics", monitoring.Handler())

	// Create server configuration
	serverConfig := &utils.ServerConfig{
		Port:         cfg.Service.Port,
		ReadTimeout:  cfg.Service.Timeout,
		WriteTimeout: cfg.Service.Timeout,
		IdleTimeout:  60 * time.Second,
	}

	// Wrap the mux with metrics (outermost) and then CORS from the router
	// Metrics must be outermost to capture all requests, including CORS-blocked ones
	handler := monitoring.HTTPMetricsMiddleware(apiRouter.ApplyCORS(mux))
	httpServer := utils.CreateServer(serverConfig, handler)

	// Flush the meter provider after the server stops, including on failure.
	// Shutdown is a no-op when metrics were never initialized.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := monitoring.Shutdown(shutdownCtx); err != nil {
			slog.Error("Metrics shutdown error", "error", err)
		}
	}()

	// Start server with graceful shutdown
	if err := utils.StartServerWithGracefulShutdown(httpServer, "consent-engine"); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
