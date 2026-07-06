package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OpenDIF/opendif-core/shared/audit"
	"github.com/gov-dx-sandbox/portal-backend/shared/utils"
	v1 "github.com/gov-dx-sandbox/portal-backend/v1"
	v1handlers "github.com/gov-dx-sandbox/portal-backend/v1/handlers"
	v1middleware "github.com/gov-dx-sandbox/portal-backend/v1/middleware"
	v1models "github.com/gov-dx-sandbox/portal-backend/v1/models"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if it exists (optional - fails silently if not found)
	_ = godotenv.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	slog.SetDefault(logger)

	slog.Info("Starting Portal Backend initialization")

	// Initialize GORM database connection for V1
	v1DbConfig := v1.NewDatabaseConfig()
	gormDB, err := v1.ConnectGormDB(v1DbConfig)
	if err != nil {
		slog.Error("Failed to connect to GORM database", "error", err)
		os.Exit(1)
	}

	// Initialize V1 handlers
	v1Handler, err := v1handlers.NewV1Handler(gormDB)
	if err != nil {
		slog.Error("Failed to initialize V1 handler", "error", err)
		os.Exit(1)
	}

	// Create a mux for API routes
	apiMux := http.NewServeMux()
	v1Handler.SetupV1Routes(apiMux) // All /api/v1/... routes go here

	// Setup middleware chain
	corsMiddleware := v1middleware.NewCORSMiddleware()

	// Setup JWT Authentication middleware
	// Validate required environment variables first
	asgardeoBaseURL := os.Getenv("ASGARDEO_BASE_URL")
	jwksURL := os.Getenv("ASGARDEO_JWKS_URL")
	issuerURL := os.Getenv("ASGARDEO_ISSUER")
	tokenURL := os.Getenv("ASGARDEO_TOKEN_URL")

	if asgardeoBaseURL == "" {
		if jwksURL == "" {
			slog.Error("ASGARDEO_JWKS_URL environment variable is required when ASGARDEO_BASE_URL is not set")
			os.Exit(1)
		}
		if issuerURL == "" && tokenURL == "" {
			slog.Error("Either ASGARDEO_ISSUER or ASGARDEO_TOKEN_URL environment variable is required when ASGARDEO_BASE_URL is not set")
			os.Exit(1)
		}
	}

	// Support multiple valid client IDs for different portals
	memberPortalClientID := os.Getenv("ASGARDEO_MEMBER_PORTAL_CLIENT_ID")
	adminPortalClientID := os.Getenv("ASGARDEO_ADMIN_PORTAL_CLIENT_ID")

	if memberPortalClientID == "" && adminPortalClientID == "" {
		slog.Error("At least one of ASGARDEO_MEMBER_PORTAL_CLIENT_ID or ASGARDEO_ADMIN_PORTAL_CLIENT_ID must be set")
		os.Exit(1)
	}

	var validClientIDs []string
	if memberPortalClientID != "" {
		validClientIDs = append(validClientIDs, memberPortalClientID)
	}
	if adminPortalClientID != "" {
		validClientIDs = append(validClientIDs, adminPortalClientID)
	}

	jwtConfig := v1middleware.JWTAuthConfig{
		JWKSURL:        utils.GetEnvOrDefault("ASGARDEO_JWKS_URL", asgardeoBaseURL+"/oauth2/jwks"),
		ExpectedIssuer: utils.GetEnvOrDefault("ASGARDEO_ISSUER", utils.GetEnvOrDefault("ASGARDEO_TOKEN_URL", asgardeoBaseURL+"/oauth2/token")),
		ValidClientIDs: validClientIDs,
		OrgName:        utils.GetEnvOrDefault("ASGARDEO_ORG_NAME", ""),
		Timeout:        10 * time.Second,
	}

	// Validate JWT configuration before proceeding
	if err := jwtConfig.Validate(); err != nil {
		slog.Error("Invalid JWT configuration", "error", err)
		os.Exit(1)
	}

	jwtAuthMiddleware := v1middleware.NewJWTAuthMiddleware(jwtConfig)

	// Setup Authorization middleware with configurable security policy
	authMode := utils.GetEnvOrDefault("AUTHORIZATION_MODE", "fail_open_admin_system")
	strictMode := utils.GetEnvOrDefault("AUTHORIZATION_STRICT_MODE", "false") == "true"

	var authConfig v1middleware.AuthorizationConfig
	switch authMode {
	case "fail_closed":
		authConfig.Mode = v1models.AuthorizationModeFailClosed
	case "fail_open_admin":
		authConfig.Mode = v1models.AuthorizationModeFailOpenAdmin
	case "fail_open_admin_system":
		authConfig.Mode = v1models.AuthorizationModeFailOpenAdminSystem
	default:
		slog.Error("Invalid authorization mode. Valid options: fail_closed, fail_open_admin, fail_open_admin_system", "mode", authMode)
		os.Exit(1)
	}
	authConfig.StrictMode = strictMode

	authorizationMiddleware := v1middleware.NewAuthorizationMiddlewareWithConfig(authConfig)

	// Initialize Audit system
	// Services will work without auditing - gracefully degrades if disabled via ENABLE_AUDIT=false
	// or if CHOREO_AUDIT_CONNECTION_SERVICEURL is not provided
	auditServiceURL := utils.GetEnvOrDefault("CHOREO_AUDIT_CONNECTION_SERVICEURL", "http://localhost:3001")
	auditClient := audit.NewClient(auditServiceURL)
	audit.InitializeGlobalAudit(auditClient)

	// Apply middleware chain (CORS -> JWT Auth -> Authorization) to the API mux ONLY
	protectedAPIHandler := corsMiddleware(
		jwtAuthMiddleware.AuthenticateJWT(
			authorizationMiddleware.AuthorizeRequest(apiMux),
		),
	)

	// Create the MAIN (top-level) mux for all incoming traffic
	topLevelMux := http.NewServeMux()

	// Register public routes directly on the top-level mux
	// These routes will bypass the audit middleware
	topLevelMux.Handle("/health", utils.PanicRecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type DBHealth struct {
			Status   string `json:"status"`
			Error    string `json:"error,omitempty"`
			Database string `json:"database,omitempty"`
		}
		type HealthStatus struct {
			Status    string              `json:"status"`
			Service   string              `json:"service"`
			Databases map[string]DBHealth `json:"databases"`
		}

		status := HealthStatus{
			Status:  "healthy",
			Service: "portal-backend",
			Databases: map[string]DBHealth{
				"v1": {Status: "unknown"},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Test V1 GORM database connection
		if gormDB == nil {
			status.Databases["v1"] = DBHealth{Status: "unhealthy", Error: "GORM connection is nil"}
			status.Status = "unhealthy"
		} else {
			sqlDB, err := gormDB.DB()
			if err != nil {
				status.Databases["v1"] = DBHealth{Status: "unhealthy", Error: fmt.Sprintf("failed to get sql.DB: %v", err)}
				status.Status = "unhealthy"
			} else if err := sqlDB.PingContext(ctx); err != nil {
				status.Databases["v1"] = DBHealth{Status: "unhealthy", Error: err.Error()}
				status.Status = "unhealthy"
			} else {
				status.Databases["v1"] = DBHealth{Status: "healthy", Database: v1DbConfig.Database}
			}
		}

		statusCode := http.StatusOK
		if status.Status != "healthy" {
			statusCode = http.StatusServiceUnavailable
		}

		utils.RespondWithJSON(w, statusCode, status)
	})))

	topLevelMux.Handle("/debug", utils.PanicRecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		utils.RespondWithJSON(w, http.StatusOK, map[string]string{"path": r.URL.Path, "method": r.Method})
	})))

	topLevelMux.Handle("/debug/db", utils.PanicRecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		debugInfo := map[string]interface{}{
			"v1": map[string]interface{}{},
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
					"error": fmt.Sprintf("failed to get sql.DB: %v", err),
				}
			} else if err := sqlDB.PingContext(ctx); err != nil {
				debugInfo["v1"] = map[string]interface{}{
					"error": fmt.Sprintf("V1 database ping failed: %v", err),
				}
			} else {
				v1Info := map[string]interface{}{
					"status":   "connected",
					"database": v1DbConfig.Database,
				}

				// Check if members table exists in V1 DB
				var membersExists bool
				checkMembersQuery := `SELECT EXISTS (
                       SELECT FROM information_schema.tables 
                       WHERE table_schema = 'public' 
                       AND table_name = 'members'
                   )`

				if err := sqlDB.QueryRowContext(ctx, checkMembersQuery).Scan(&membersExists); err != nil {
					v1Info["table_check_error"] = fmt.Sprintf("failed to check members table: %v", err)
				} else {
					v1Info["members_table_exists"] = membersExists
					if membersExists {
						var memberCount int
						countMembersQuery := `SELECT COUNT(*) FROM members`
						if err := sqlDB.QueryRowContext(ctx, countMembersQuery).Scan(&memberCount); err != nil {
							v1Info["count_error"] = fmt.Sprintf("failed to count members: %v", err)
						} else {
							v1Info["members_count"] = memberCount
						}
					}
				}
				debugInfo["v1"] = v1Info
			}
		}

		utils.RespondWithJSON(w, http.StatusOK, debugInfo)
	})))

	// Register the protected API routes to the top-level mux
	// All traffic to /api/v1/ (and its sub-paths) will pass through the middleware chain
	topLevelMux.Handle("/api/v1/", protectedAPIHandler)

	// Register internal API routes (no authentication required for internal services)
	// SECURITY WARNING: These endpoints are exposed WITHOUT authentication!
	// MUST be protected at network level (VPC, firewall, service mesh, etc.)
	// See README.md "Deployment Security" section for required security measures.
	// DO NOT expose this service directly to public internet without proper network isolation.
	topLevelMux.Handle("/internal/api/v1/", http.StripPrefix("", apiMux))

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	addr := ":" + port
	server := &http.Server{
		Addr:         addr,
		Handler:      topLevelMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("Portal Backend starting", "port", port, "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start Portal Backend", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down Portal Backend...")

	// Create a deadline to wait for
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	// Gracefully close database connection
	if gormDB != nil {
		if sqlDB, err := gormDB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				slog.Error("Failed to close database connection", "error", err)
			}
		}
	}

	slog.Info("Portal Backend exited")
}
