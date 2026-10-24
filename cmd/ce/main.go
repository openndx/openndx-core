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

	"github.com/OpenNSW/core/cors"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/ce/handlers"
	"github.com/openndx/openndx-core/internal/ce/middleware"
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
	cfg, err := loadConfig(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Setup logging
	utils.SetupLogging(cfg.Logging.Format, cfg.Logging.Level)

	slog.Info("Starting consent engine",
		"port", cfg.Server.Port,
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit)

	// Temporary GORM initialization until #631 switches to database.Open.
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

	slog.Info("Using consent portal URL", "url", cfg.ConsentPortalURL, "cors_allowed_origins", cfg.CORS.AllowedOrigins)

	// Initialize consent service
	consentService, err := services.NewConsentService(gormDB, cfg.ConsentPortalURL)
	if err != nil {
		return fmt.Errorf("initialize consent service: %w", err)
	}

	// Initialize handlers
	internalHandler := handlers.NewInternalHandler(consentService)
	portalHandler := handlers.NewPortalHandler(consentService)

	slog.Info("Portal authentication configuration",
		"issuer", cfg.IDP.Issuer,
		"audience", cfg.IDP.Audience,
		"jwks_url", cfg.IDP.JWKSURL,
		"subject_claim", cfg.IDP.SubjectClaim,
		"jwks_insecure_skip_verify", cfg.IDP.InsecureSkipTLSVerify)

	// Authenticate consent-portal requests: authn validates the token, then
	// OwnerSubject reads the data owner's UID from the configured claim.
	authnManager, err := authn.NewManager(cfg.IDP.Config)
	if err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}
	requireAuth := authnManager.RequireAuthMiddleware()
	ownerSubject := middleware.OwnerSubject(cfg.IDP.SubjectClaim)
	authenticate := func(next http.Handler) http.Handler {
		return requireAuth(ownerSubject(next))
	}

	// Initialize router and register all routes
	apiRouter := router.NewRouter(internalHandler, portalHandler, authenticate)
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
		Port:         strconv.Itoa(cfg.Server.Port),
		ReadTimeout:  cfg.Server.Timeout,
		WriteTimeout: cfg.Server.Timeout,
		IdleTimeout:  60 * time.Second,
	}

	// Wrap the mux with metrics (outermost) and then CORS
	// Metrics must be outermost to capture all requests, including CORS-blocked ones
	handler := monitoring.HTTPMetricsMiddleware(cors.CORS(&cfg.CORS)(mux))
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
