package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/configs"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/federator"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/middleware"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/provider"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/server"
)

func main() {
	logger.Init()

	// Create context with signal handling for graceful shutdown
	// This ensures background goroutines (like JWKS refresh) are properly cancelled
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Load configuration with proper error handling
	config, err := configs.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize audit middleware
	// Allow environment variable override for runtime configuration
	envAuditURL := os.Getenv("AUDIT_SERVICE_URL")
	if envAuditURL != "" {
		config.AuditConfig.ServiceURL = envAuditURL
	}
	apiKey := os.Getenv("ARGUS_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ARGUS_AUTH_TOKEN")
	}
	auditClient := audit.NewClient(audit.Config{
		BaseURL: config.AuditConfig.ServiceURL,
		APIKey:  apiKey,
	})
	audit.InitializeGlobalAudit(auditClient)

	// Initialize audit configuration (actorType, actorID)
	// Note: targetType is determined per API call, not from global config
	middleware.InitializeAuditConfig(
		config.AuditConfig.ActorType,
		config.AuditConfig.ActorID,
	)

	providerHandler := provider.NewProviderHandler(config.GetProviders())

	federationObject, err := federator.Initialize(ctx, config, providerHandler, nil)
	if err != nil {
		log.Fatalf("Failed to initialize federator: %v", err)
	}

	// Run server with graceful shutdown support
	// Server will stop when ctx is cancelled (on SIGINT/SIGTERM)
	server.RunServer(ctx, federationObject)
}
