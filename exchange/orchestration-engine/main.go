package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNDX/openndx-core/exchange/shared/monitoring"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/configs"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/federator"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/middleware"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/provider"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/server"
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

	// Initialize monitoring
	var tracker monitoring.Tracker = monitoring.NewNoOpTracker()
	if monitoring.IsObservabilityEnabled() {
		monitoringConfig := monitoring.DefaultConfig("orchestration-engine")
		if err := monitoring.Initialize(monitoringConfig); err != nil {
			log.Printf("Failed to initialize monitoring (service will continue): %v", err)
		} else {
			tracker = monitoring.NewOTelTracker()
			log.Printf("Monitoring initialized successfully for orchestration-engine")
		}
	}

	providerHandler := provider.NewProviderHandler(config.GetProviders())

	federationObject, err := federator.Initialize(ctx, config, providerHandler, nil, tracker)
	if err != nil {
		log.Fatalf("Failed to initialize federator: %v", err)
	}

	// Run server with graceful shutdown support
	// Server will stop when ctx is cancelled (on SIGINT/SIGTERM)
	server.RunServer(ctx, federationObject)
}
