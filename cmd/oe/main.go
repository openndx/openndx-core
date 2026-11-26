// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

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

	"github.com/OpenNSW/core/telemetry"
	"github.com/openndx/openndx-core/internal/audit"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/logging"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator"
	"github.com/openndx/openndx-core/internal/oe/federator/schema"
	"github.com/openndx/openndx-core/internal/oe/middleware"
	"github.com/openndx/openndx-core/internal/oe/policy"
	"github.com/openndx/openndx-core/internal/oe/provider"
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
		slog.Error("Orchestration engine failed", "error", err)
		os.Exit(1)
	}
}

// run loads validated configuration and owns the orchestration engine's server.
func run() error {
	config, err := loadConfig(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logging.Setup(config.Logging, "orchestration-engine", config.Environment, Version)

	slog.Info("Starting orchestration engine",
		"port", config.Server.Port,
		"build_time", BuildTime,
		"git_commit", GitCommit)

	tel, err := telemetry.Start(context.Background(), config.Otel)
	if err != nil {
		return fmt.Errorf("start telemetry (check otel in the config file): %w", err)
	}
	// Runs after the server has stopped, so the last audit records are flushed.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tel.Shutdown(ctx); err != nil {
			slog.Error("Failed to shut down telemetry", "error", err)
		}
	}()
	auditSink := audit.NewExportSink(tel.SlogHandler())

	// The consumer schema is read once at startup. To change it, deploy a new
	// schema file and restart, so every OE instance serves the same schema.
	consumerSchema, err := schema.Load(config.SchemaPath)
	if err != nil {
		return fmt.Errorf("load the consumer schema (check schemaPath in the config file): %w", err)
	}
	slog.Info("Loaded consumer schema", "path", config.SchemaPath)

	// Authenticate consumers: accept tokens the IdP issued for OE's audience,
	// whichever client requested them.
	authnManager, err := authn.NewManager(config.IDP)
	if err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}

	providers, err := provider.NewRegistry(config.Providers...)
	if err != nil {
		return fmt.Errorf("invalid provider configuration: %w", err)
	}

	deps := federator.Deps{
		Providers: federator.ProviderLookup(providers.Get),
		Schemas:   consumerSchema,
		Auditor:   middleware.NewAuditor(auditSink, config.Audit.ActorType, config.Audit.ActorID),
	}
	if config.PDP.URL != "" {
		deps.Policy = policy.NewPdpClient(config.PDP.URL)
	}
	if config.ConsentEngine.URL != "" {
		deps.Consent = consent.NewCEServiceClient(config.ConsentEngine.URL)
	}

	federationObject, err := federator.New(deps, federator.Options{
		OwnerIDArgName: config.ConsentEngine.OwnerIDArgName,
		ArgMapping:     config.ArgMappings,
	})
	if err != nil {
		return fmt.Errorf("initialize federator: %w", err)
	}

	mux := http.NewServeMux()
	// Consumer endpoint: requires a token the IdP issued for OE's audience.
	mux.Handle("POST /public/graphql", authnManager.RequireAuthMiddleware()(federator.Handler(federationObject)))
	mux.Handle("GET /health", utils.HealthHandler("orchestration-engine"))

	httpServer := utils.CreateServer(&utils.ServerConfig{
		Port:         strconv.Itoa(config.Server.Port),
		ReadTimeout:  config.Server.Timeout,
		WriteTimeout: config.Server.Timeout,
		IdleTimeout:  60 * time.Second,
	}, mux)

	// Serves until SIGINT or SIGTERM, then shuts down gracefully.
	if err := utils.StartServerWithGracefulShutdown(httpServer); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}
