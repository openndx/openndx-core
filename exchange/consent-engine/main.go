package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/shared/monitoring"
	"github.com/OpenNDX/openndx-core/exchange/shared/utils"
	"github.com/gov-dx-sandbox/exchange/consent-engine/internal/config"

	// V1 API imports
	v1auth "github.com/gov-dx-sandbox/exchange/consent-engine/v1/auth"
	v1db "github.com/gov-dx-sandbox/exchange/consent-engine/v1/database"
	v1handlers "github.com/gov-dx-sandbox/exchange/consent-engine/v1/handlers"
	v1router "github.com/gov-dx-sandbox/exchange/consent-engine/v1/router"
	v1services "github.com/gov-dx-sandbox/exchange/consent-engine/v1/services"
)

// Build information - set during build
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Load configuration using flags
	cfg := config.LoadConfig("consent-engine")

	// Setup logging
	utils.SetupLogging(cfg.Logging.Format, cfg.Logging.Level)

	// Initialize monitoring/observability (optional - can be disabled via ENABLE_OBSERVABILITY=false)
	// Services will continue to function normally even if observability is disabled
	if monitoring.IsObservabilityEnabled() {
		monitoringConfig := monitoring.DefaultConfig("consent-engine")
		if err := monitoring.Initialize(monitoringConfig); err != nil {
			slog.Warn("Failed to initialize monitoring (service will continue)", "error", err)
		}
	} else {
		slog.Info("Observability disabled via environment variable")
	}

	slog.Info("Starting consent engine",
		"environment", cfg.Environment,
		"port", cfg.Service.Port,
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit)

	// Initialize V1 database connection
	slog.Info("Initializing V1 database connection...")
	v1DBConfig := v1db.NewDatabaseConfig(&cfg.DBConfigs)
	v1DB, err := v1db.ConnectGormDB(v1DBConfig)
	if err != nil {
		slog.Error("Failed to connect to V1 database", "error", err)
		os.Exit(1)
	}
	slog.Info("V1 database connected successfully")

	// Get underlying SQL DB for proper cleanup
	v1SqlDB, err := v1DB.DB()
	if err != nil {
		slog.Error("Failed to get V1 database connection", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := v1SqlDB.Close(); err != nil {
			slog.Error("Failed to close V1 database connection", "error", err)
		} else {
			slog.Info("V1 database connection closed successfully")
		}
	}()

	// Get consent portal URL from environment
	slog.Info("Using consent portal URL", "url", cfg.ConsentPortalUrl)

	// Initialize V1 consent service
	v1ConsentService, err := v1services.NewConsentService(v1DB, cfg.ConsentPortalUrl)
	if err != nil {
		slog.Error("Failed to initialize V1 consent service", "error", err)
		os.Exit(1)
	}

	// Initialize V1 handlers
	v1InternalHandler := v1handlers.NewInternalHandler(v1ConsentService)
	v1PortalHandler := v1handlers.NewPortalHandler(v1ConsentService)

	slog.Info("JWT verifier configuration",
		"client_id", cfg.IDPConfig.ClientID,
		"issuer", cfg.IDPConfig.Issuer,
		"audience", cfg.IDPConfig.Audience,
		"jwks_url", cfg.IDPConfig.JwksUrl,
		"jwks_insecure_skip_verify", cfg.IDPConfig.InsecureSkipVerify)

	v1JWTVerifier, err := v1auth.NewJWTVerifier(v1auth.JWTVerifierConfig{
		JWKSUrl:            cfg.IDPConfig.JwksUrl,
		Issuer:             cfg.IDPConfig.Issuer,
		Audience:           cfg.IDPConfig.Audience,
		ClientID:           cfg.IDPConfig.ClientID,
		InsecureSkipVerify: cfg.IDPConfig.InsecureSkipVerify,
	})
	if err != nil {
		slog.Error("Failed to initialize V1 JWT verifier", "error", err)
		os.Exit(1)
	}

	// Initialize V1 router and register all V1 routes
	v1Router := v1router.NewV1Router(cfg.Service.AllowedOrigins, v1InternalHandler, v1PortalHandler, v1JWTVerifier)
	mux := http.NewServeMux()

	slog.Info("Registering V1 API routes")
	v1Router.RegisterRoutes(mux)
	slog.Info("V1 API routes registered successfully")

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

	// Wrap the mux with metrics (outermost) and then CORS from v1 router
	// Metrics must be outermost to capture all requests, including CORS-blocked ones
	handler := monitoring.HTTPMetricsMiddleware(v1Router.ApplyCORS(mux))
	httpServer := utils.CreateServer(serverConfig, handler)

	// Start server with graceful shutdown
	if err := utils.StartServerWithGracefulShutdown(httpServer, "consent-engine"); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
