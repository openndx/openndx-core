package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNSW/core/authz"
	"github.com/OpenNSW/core/cors"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/database"
	"github.com/openndx/openndx-core/internal/logging"
	"github.com/openndx/openndx-core/internal/pb/application"
	"github.com/openndx/openndx-core/internal/pb/caller"
	"github.com/openndx/openndx-core/internal/pb/consentbypass"
	"github.com/openndx/openndx-core/internal/pb/idp/thunderid"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/openndx/openndx-core/internal/pb/schema"
	"github.com/openndx/openndx-core/internal/utils"
)

// Build information - set during build
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	cfg, err := loadConfig(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	logging.Setup(cfg.Logging, "portal-backend", cfg.Environment, Version)

	slog.Info("Starting Portal Backend initialization",
		"build_time", BuildTime,
		"git_commit", GitCommit)

	gormDB, sqlDB, err := database.OpenGORM(context.Background(), cfg.Database.Config, cfg.Database.IdleTimeout)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	dbName := cfg.Database.Postgres.Name
	slog.Info("Connected to PostgreSQL",
		"host", cfg.Database.Postgres.Host,
		"port", cfg.Database.Postgres.Port,
		"database", dbName)

	// Initialize external service clients
	idpProvider := thunderid.NewClient(cfg.IDP.Management)
	slog.Info("PDP Service URL", "url", cfg.PDP.URL)
	pdpClient := policy.NewClient(cfg.PDP.URL)

	// Initialize Audit system
	// Services will work without auditing - gracefully degrades if audit.url is
	// empty (or the audit library's ENABLE_AUDIT=false is set)
	auditClient := audit.NewClient(audit.Config{
		BaseURL: cfg.Audit.URL,
		APIKey:  cfg.Audit.APIKey,
	})
	audit.InitializeGlobalAudit(auditClient)

	// Initialize services and handlers
	memberService := member.NewService(gormDB)
	memberHandler := member.NewHandler(memberService)

	schemaService := schema.NewService(gormDB, pdpClient)
	schemaHandler := schema.NewHandler(schemaService)

	applicationService := application.NewService(gormDB, pdpClient, idpProvider)
	applicationHandler := application.NewHandler(applicationService)

	consentBypassService := consentbypass.NewService(applicationService, pdpClient)
	consentBypassHandler := consentbypass.NewHandler(consentBypassService)

	// Setup authentication: accept tokens the IdP issued for this API's
	// audience, whichever client requested them.
	authnManager, err := authn.NewManager(cfg.IDP.Config)
	if err != nil {
		slog.Error("Failed to initialize authentication", "error", err)
		os.Exit(1)
	}

	// Check each API route's OAuth2 scope against the principal internal/authn validated
	authzr, err := authz.New(caller.PrincipalFromContext)
	if err != nil {
		slog.Error("Failed to initialize authorization", "error", err)
		os.Exit(1)
	}

	// API routes. Each requires one scope: the "-all" scopes never open a
	// route, handlers check them only to widen the caller past its own records.
	apiMux := http.NewServeMux()

	// Member endpoints
	apiMux.Handle("GET /api/v1/members", authzr.RequireScope(caller.ScopeMemberRead)(http.HandlerFunc(memberHandler.GetAllMembers)))
	apiMux.Handle("POST /api/v1/members", authzr.RequireScope(caller.ScopeMemberWriteAll)(http.HandlerFunc(memberHandler.CreateMember)))
	apiMux.Handle("GET /api/v1/members/{memberId}", authzr.RequireScope(caller.ScopeMemberRead)(http.HandlerFunc(memberHandler.GetMember)))
	apiMux.Handle("PUT /api/v1/members/{memberId}", authzr.RequireScope(caller.ScopeMemberWrite)(http.HandlerFunc(memberHandler.UpdateMember)))

	// Schema endpoints
	apiMux.Handle("GET /api/v1/schemas", authzr.RequireScope(caller.ScopeSchemaRead)(http.HandlerFunc(schemaHandler.GetAllSchemas)))
	apiMux.Handle("POST /api/v1/schemas", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.CreateSchema)))
	apiMux.Handle("GET /api/v1/schemas/{schemaId}", authzr.RequireScope(caller.ScopeSchemaRead)(http.HandlerFunc(schemaHandler.GetSchema)))
	apiMux.Handle("PUT /api/v1/schemas/{schemaId}", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.UpdateSchema)))

	// Schema policy metadata endpoints
	apiMux.Handle("GET /api/v1/schemas/{schemaId}/policy-metadata", authzr.RequireScope(caller.ScopeSchemaRead)(http.HandlerFunc(schemaHandler.ListSchemaPolicyMetadata)))
	apiMux.Handle("PATCH /api/v1/schemas/{schemaId}/policy-metadata/{id}", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.PatchSchemaPolicyMetadata)))
	apiMux.Handle("DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.DeleteSchemaPolicyMetadata)))
	apiMux.Handle("DELETE /api/v1/schemas/{schemaId}/policy-metadata/{id}/allowlist/{applicationId}", authzr.RequireScope(caller.ScopeSchemaWrite)(http.HandlerFunc(schemaHandler.RevokeSchemaPolicyAllowListEntry)))

	// Schema submission endpoints
	apiMux.Handle("GET /api/v1/schema-submissions", authzr.RequireScope(caller.ScopeSchemaSubmissionRead)(http.HandlerFunc(schemaHandler.GetAllSchemaSubmissions)))
	apiMux.Handle("POST /api/v1/schema-submissions", authzr.RequireScope(caller.ScopeSchemaSubmissionWrite)(http.HandlerFunc(schemaHandler.CreateSchemaSubmission)))
	apiMux.Handle("GET /api/v1/schema-submissions/{submissionId}", authzr.RequireScope(caller.ScopeSchemaSubmissionRead)(http.HandlerFunc(schemaHandler.GetSchemaSubmission)))
	apiMux.Handle("PUT /api/v1/schema-submissions/{submissionId}", authzr.RequireScope(caller.ScopeSchemaSubmissionWrite)(http.HandlerFunc(schemaHandler.UpdateSchemaSubmission)))

	// Application endpoints
	apiMux.Handle("GET /api/v1/applications", authzr.RequireScope(caller.ScopeApplicationRead)(http.HandlerFunc(applicationHandler.GetAllApplications)))
	apiMux.Handle("POST /api/v1/applications", authzr.RequireScope(caller.ScopeApplicationWrite)(http.HandlerFunc(applicationHandler.CreateApplication)))
	apiMux.Handle("GET /api/v1/applications/{applicationId}", authzr.RequireScope(caller.ScopeApplicationRead)(http.HandlerFunc(applicationHandler.GetApplication)))
	apiMux.Handle("PUT /api/v1/applications/{applicationId}", authzr.RequireScope(caller.ScopeApplicationWrite)(http.HandlerFunc(applicationHandler.UpdateApplication)))
	apiMux.Handle("PUT /api/v1/applications/{applicationId}/policy", authzr.RequireScope(caller.ScopeApplicationWrite)(http.HandlerFunc(applicationHandler.UpdateApplicationPolicy)))

	// Consent bypass grant endpoints
	apiMux.Handle("POST /api/v1/applications/{applicationId}/consent-bypass-grants", authzr.RequireScope(caller.ScopeConsentBypassWrite)(http.HandlerFunc(consentBypassHandler.Grant)))
	apiMux.Handle("GET /api/v1/consent-bypass-grants", authzr.RequireScope(caller.ScopeConsentBypassRead)(http.HandlerFunc(consentBypassHandler.List)))
	apiMux.Handle("POST /api/v1/consent-bypass-grants/{grantId}/revoke", authzr.RequireScope(caller.ScopeConsentBypassWrite)(http.HandlerFunc(consentBypassHandler.Revoke)))

	// Application submission endpoints
	apiMux.Handle("GET /api/v1/application-submissions", authzr.RequireScope(caller.ScopeApplicationSubmissionRead)(http.HandlerFunc(applicationHandler.GetAllApplicationSubmissions)))
	apiMux.Handle("POST /api/v1/application-submissions", authzr.RequireScope(caller.ScopeApplicationSubmissionWrite)(http.HandlerFunc(applicationHandler.CreateApplicationSubmission)))
	apiMux.Handle("GET /api/v1/application-submissions/{submissionId}", authzr.RequireScope(caller.ScopeApplicationSubmissionRead)(http.HandlerFunc(applicationHandler.GetApplicationSubmission)))
	apiMux.Handle("PUT /api/v1/application-submissions/{submissionId}", authzr.RequireScope(caller.ScopeApplicationSubmissionWrite)(http.HandlerFunc(applicationHandler.UpdateApplicationSubmission)))

	// All incoming traffic. Public routes register here directly; everything
	// under /api/v1/ goes through CORS, authentication and the caller's member
	// lookup to apiMux. The
	// method-less /api/v1/ pattern also catches CORS preflight (OPTIONS)
	// requests, which no apiMux route matches.
	mux := http.NewServeMux()

	mux.Handle("GET /health", utils.HealthHandler("portal-backend"))
	mux.HandleFunc("GET /debug", debugHandler)
	mux.HandleFunc("GET /debug/db", debugDBHandler(gormDB, dbName))

	mux.Handle("/api/v1/", cors.CORS(&cfg.CORS)(authnManager.RequireAuthMiddleware()(caller.Middleware(memberService.MemberIDByOuID)(apiMux))))

	// Start server
	port := strconv.Itoa(cfg.Server.Port)
	addr := ":" + port
	server := &http.Server{
		Addr:         addr,
		Handler:      utils.PanicRecoveryMiddleware(mux),
		ReadTimeout:  cfg.Server.Timeout,
		WriteTimeout: cfg.Server.Timeout,
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

	if err := sqlDB.Close(); err != nil {
		slog.Error("Failed to close database connection", "error", err)
	}

	slog.Info("Portal Backend exited")
}
