package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/shared/monitoring"
	"github.com/OpenNDX/openndx-core/exchange/shared/utils"
	"github.com/gov-dx-sandbox/exchange/policy-decision-point/internal/config"
	v1 "github.com/gov-dx-sandbox/exchange/policy-decision-point/v1"
)

// Build information - set during build
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Load configuration using flags
	cfg := config.LoadConfig("policy-decision-point")

	// Setup logging
	utils.SetupLogging(cfg.Logging.Format, cfg.Logging.Level)

	// Initialize monitoring/observability (optional - can be disabled via ENABLE_OBSERVABILITY=false)
	// Services will continue to function normally even if observability is disabled
	if monitoring.IsObservabilityEnabled() {
		monitoringConfig := monitoring.DefaultConfig("policy-decision-point")
		if err := monitoring.Initialize(monitoringConfig); err != nil {
			slog.Warn("Failed to initialize monitoring (service will continue)", "error", err)
		}
	} else {
		slog.Info("Observability disabled via environment variable")
	}

	slog.Info("Starting policy decision point",
		"environment", cfg.Environment,
		"port", cfg.Service.Port,
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit)

	// Log database configuration being used
	slog.Info("Database configuration",
		"host", cfg.DBConfigs.Host,
		"port", cfg.DBConfigs.Port,
		"username", cfg.DBConfigs.Username,
		"database", cfg.DBConfigs.Database,
		"sslmode", cfg.DBConfigs.SSLMode)

	// Log IDP configuration (for future use)
	slog.Info("IDP configuration",
		"issuer", cfg.IDPConfig.Issuer,
		"audience", cfg.IDPConfig.Audience,
		"jwks_url", cfg.IDPConfig.JwksURL)

	// Initialize V1 GORM database connection
	v1DbConfig := v1.NewDatabaseConfig(&cfg.DBConfigs)
	gormDB, err := v1.ConnectGormDB(v1DbConfig)
	if err != nil {
		slog.Error("Failed to connect to GORM database", "error", err)
		os.Exit(1)
	}
	slog.Info("V1 database connected successfully")

	// Get underlying SQL DB for proper cleanup
	v1SqlDB, err := gormDB.DB()
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

	// Monitor database connection pool metrics (USE metrics)
	if monitoring.IsObservabilityEnabled() {
		if err := monitoring.MonitorDBConnectionPool(v1SqlDB); err != nil {
			slog.Warn("Failed to monitor database connection pool", "error", err)
		} else {
			slog.Info("Database connection pool metrics registered successfully")
		}
	}

	// Initialize V1 handlers
	v1Handler := v1.NewHandler(gormDB)

	// Setup routes
	mux := http.NewServeMux()
	v1Handler.SetupRoutes(mux) // V1 routes with /api/v1/policy/ prefix

	// Metrics endpoint
	mux.Handle("/metrics", monitoring.Handler())

	// Health check endpoint
	mux.Handle("/health", utils.PanicRecoveryMiddleware(utils.HealthHandler("policy-decision-point")))

	// Debug endpoint
	mux.Handle("/debug", utils.PanicRecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		utils.RespondWithJSON(w, http.StatusOK, map[string]string{
			"service": "policy-decision-point",
			"version": Version,
			"path":    r.URL.Path,
			"method":  r.Method,
		})
	})))

	// Database debug endpoint
	mux.Handle("/debug/db", utils.PanicRecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					"database": v1DbConfig.Database,
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
	})))

	// Wrap with metrics middleware
	handler := monitoring.HTTPMetricsMiddleware(mux)

	// Create server configuration
	serverConfig := &utils.ServerConfig{
		Port:         cfg.Service.Port,
		ReadTimeout:  cfg.Service.Timeout,
		WriteTimeout: cfg.Service.Timeout,
		IdleTimeout:  60 * time.Second,
	}
	server := utils.CreateServer(serverConfig, handler)

	// Start server with graceful shutdown
	if err := utils.StartServerWithGracefulShutdown(server, "policy-decision-point"); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
