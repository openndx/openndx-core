// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package authn

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Manager handles all authentication-related operations and middleware setup.
// It encapsulates the token extraction and middleware creation,
// providing a clean interface for the HTTP server to use.
//
// The manager only authenticates: it validates the token and injects the
// resulting Principal. Anything an application does with that principal (e.g.
// provisioning a user record) belongs in its own middleware chained after it.
type Manager struct {
	tokenExtractor *TokenExtractor
	middleware     func(http.Handler) http.Handler
}

// NewManager creates and initializes a new auth manager.
// This is the single entry point for all auth initialization in the application.
//
// Usage:
//
//	authManager, err := auth.NewManager(cfg.Auth)
func NewManager(authConfig Config) (*Manager, error) {
	slog.Info("initializing auth manager")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	if authConfig.InsecureSkipTLSVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // opt-in insecureSkipTLSVerify for local IdPs with self-signed certificates; off by default
		}
	}

	tokenExtractor, err := NewTokenExtractorWithClient(
		authConfig.JWKSURL, authConfig.Issuer, authConfig.Audience, httpClient,
		buildOptions(authConfig)...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize token extractor: %w", err)
	}
	if tokenExtractor == nil {
		return nil, fmt.Errorf("token extractor not initialized")
	}

	return &Manager{
		tokenExtractor: tokenExtractor,
		middleware:     Middleware(tokenExtractor),
	}, nil
}

// buildOptions translates Config's optional fields into TokenExtractor
// Options, so Config-driven (NewManager) and direct (NewTokenExtractor)
// callers share identical semantics.
func buildOptions(cfg Config) []Option {
	return []Option{WithClaims(cfg.Claims)}
}

// Middleware returns the auth middleware function.
// This middleware extracts and injects authentication context into requests.
//
// Usage:
//
//	handler := authManager.Middleware()(mux)
//
// The middleware:
// 1. Extracts Authorization header
// 2. Parses token into a Principal
// 3. Injects context into request
func (m *Manager) Middleware() func(http.Handler) http.Handler { return m.middleware }

// RequireAuthMiddleware returns a middleware that requires authentication.
// If no auth context is found, returns 401 Unauthorized.
// Use this for protected endpoints.
//
// Usage:
//
//	mux.Handle("POST /api/protected",
//	    authManager.RequireAuthMiddleware()(handler),
//	)
func (m *Manager) RequireAuthMiddleware() func(http.Handler) http.Handler {
	return RequireAuth(m.tokenExtractor)
}

// OptionalAuthMiddleware returns a middleware for endpoints that work with or without auth.
// This is the same as the basic Middleware() - included for semantic clarity.
//
// Usage:
//
//	mux.Handle("GET /api/products",
//	    authManager.OptionalAuthMiddleware()(handler),
//	)
//
// The handler can check if auth context is available and personalize response.
func (m *Manager) OptionalAuthMiddleware() func(http.Handler) http.Handler { return m.middleware }

// Health checks if the auth system is functioning properly.
// It verifies that the auth system components are initialized correctly.
//
// Usage in server startup:
//
//	authManager := auth.NewManager(cfg.Auth)
//	if err := authManager.Health(); err != nil {
//	    log.Fatalf("auth system health check failed: %v", err)
//	}
//
// Returns an error if auth system components are misconfigured.
func (m *Manager) Health() error {
	if m.tokenExtractor == nil {
		return fmt.Errorf("token extractor not initialized")
	}
	return nil
}

// Close performs any cleanup needed for the auth manager.
// Currently a no-op, but provided for future extensibility
// (e.g., closing JWT validators, flushing caches, logging stats).
//
// Usage:
//
//	authManager := auth.NewManager(cfg.Auth)
//	defer authManager.Close()
func (m *Manager) Close() error {
	slog.Debug("auth manager closing")
	return nil
}
