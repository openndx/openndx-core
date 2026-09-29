# OpenNDX - Comprehensive Makefile
# This Makefile provides standardized commands for all services in the repository

.PHONY: help install-hooks setup validate-build validate-test validate-docker-build check-lint run clean setup-all validate-build-all validate-test-all

# Default target
help:
	@echo "OpenNDX - Available Commands"
	@echo "=========================================="
	@echo ""
	@echo "Usage: make [COMMAND] [SERVICE]"
	@echo ""
	@echo "Services:"
	@echo "  Go Services:"
	@echo "    - portal-backend"
	@echo "    - orchestration-engine"
	@echo "    - consent-engine"
	@echo "    - policy-decision-point"
	@echo ""
	@echo "  Frontend Services:"
	@echo "    - member-portal"
	@echo "    - admin-portal"
	@echo "    - consent-portal"
	@echo ""
	@echo "Commands:"
	@echo "  setup [SERVICE]                - Install required modules/dependencies"
	@echo "  validate-build [SERVICE]       - Build the service and validate"
	@echo "  validate-test [SERVICE]        - Run unit tests with coverage"
	@echo "  validate-docker-build [SERVICE] - Validate Docker image builds"
	@echo "  check-lint [SERVICE]           - Run lint checks"
	@echo "  quality-check [SERVICE]        - Run all quality checks (format, lint, security)"
	@echo "  format [SERVICE]               - Format Go code with gofumpt and goimports"
	@echo "  lint [SERVICE]                 - Run lint checks (go vet, gofmt)"
	@echo "  security [SERVICE]             - Run security checks with gosec"
	@echo "  staticcheck [SERVICE]          - Run staticcheck analysis"
	@echo "  install-tools                  - Install all required Go quality tools"
	@echo "  run [SERVICE]                  - Run the service locally"
	@echo "  clean                          - Clean all build artifacts"
	@echo ""
	@echo "Batch Commands:"
	@echo "  setup-all                      - Setup all services"
	@echo "  validate-build-all             - Build all services"
	@echo "  validate-test-all              - Test all services"
	@echo "  quality-check-all              - Run quality checks on all Go services"
	@echo "  format-all                     - Format all Go services"
	@echo "  lint-all                       - Lint all Go services"
	@echo ""
	@echo "Examples:"
	@echo "  make setup portal-backend"
	@echo "  make validate-build orchestration-engine"
	@echo "  make validate-test consent-engine"
	@echo "  make run member-portal"

# Variables
ROOT_DIR := $(shell pwd)
BIN_DIR := $(ROOT_DIR)/bin
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Service paths

# Consent Engine, Orchestration Engine, Policy Decision Point, and Portal
# Backend are part of the root Go module (see go.mod): their binary
# entrypoints live under cmd/<svc>, their implementation under internal/<svc>.
# Because they aren't self-contained module directories like the services
# above, they use the *-go-rootmod-service targets below instead of
# *-go-service.
CONSENT_ENGINE_PATH := cmd/ce
CONSENT_ENGINE_INTERNAL_PATH := internal/ce
CONSENT_ENGINE_TEST_PATH := ./cmd/ce/... ./internal/ce/...
CONSENT_ENGINE_DOCKERFILE := cmd/ce/Dockerfile

ORCHESTRATION_ENGINE_PATH := cmd/oe
ORCHESTRATION_ENGINE_INTERNAL_PATH := internal/oe
ORCHESTRATION_ENGINE_TEST_PATH := ./cmd/oe/... ./internal/oe/...
ORCHESTRATION_ENGINE_DOCKERFILE := cmd/oe/Dockerfile

POLICY_DECISION_POINT_PATH := cmd/pdp
POLICY_DECISION_POINT_INTERNAL_PATH := internal/pdp
POLICY_DECISION_POINT_TEST_PATH := ./cmd/pdp/... ./internal/pdp/...
POLICY_DECISION_POINT_DOCKERFILE := cmd/pdp/Dockerfile

portal_backend_PATH := cmd/pb
PORTAL_BACKEND_INTERNAL_PATH := internal/pb
PORTAL_BACKEND_TEST_PATH := ./cmd/pb/... ./internal/pb/...
PORTAL_BACKEND_DOCKERFILE := cmd/pb/Dockerfile

MEMBER_PORTAL_PATH := portals/apps/member
ADMIN_PORTAL_PATH := portals/apps/admin
CONSENT_PORTAL_PATH := portals/apps/consent

# Go services list
GO_SERVICES := portal-backend orchestration-engine consent-engine policy-decision-point
FRONTEND_SERVICES := member-portal admin-portal consent-portal

# =============================================================================
# ROUTER HELPER FUNCTIONS
# =============================================================================

# Common shell function to resolve service paths and route commands
# This consolidates the repeated pattern across all router targets

# Create bin directory
$(BIN_DIR):
	@mkdir -p $(BIN_DIR)

# =============================================================================
# SETUP COMMANDS
# =============================================================================

# Install Git hooks
install-hooks:
	@echo "Installing git hooks..."
	@if [ ! -d ".githooks" ]; then \
		echo "❌ Error: .githooks directory not found"; \
		exit 1; \
	fi
	@HOOKS_DIR=$$(git rev-parse --git-path hooks 2>/dev/null || echo ".git/hooks") && \
	mkdir -p "$$HOOKS_DIR" && \
	cp .githooks/pre-commit "$$HOOKS_DIR/pre-commit" && \
	chmod +x "$$HOOKS_DIR/pre-commit"
	@echo "✅ Git hooks installed successfully"
	@echo "📍 Pre-commit hook will now run automatically on every commit"
	@echo "💡 To bypass temporarily, use: git commit --no-verify"
	
# Setup for Go services
setup-go-service:
	@echo "Setting up Go service: $(SERVICE)"
	@cd $(SERVICE_PATH) && go mod tidy
	@cd $(SERVICE_PATH) && go mod download
	@echo "✅ Go service $(SERVICE) dependencies installed"

# Setup for Frontend services
setup-frontend-service:
	@echo "Setting up Frontend service: $(SERVICE)"
	@cd $(SERVICE_PATH) && pnpm install --frozen-lockfile
	@echo "✅ Frontend service $(SERVICE) dependencies installed"

# Setup command router
setup:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; TARGET="setup-go-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; TARGET="setup-go-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; TARGET="setup-go-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; TARGET="setup-go-service" ;; \
		member-portal) SERVICE_PATH="$(MEMBER_PORTAL_PATH)"; TARGET="setup-frontend-service" ;; \
		admin-portal) SERVICE_PATH="$(ADMIN_PORTAL_PATH)"; TARGET="setup-frontend-service" ;; \
		consent-portal) SERVICE_PATH="$(CONSENT_PORTAL_PATH)"; TARGET="setup-frontend-service" ;; \
		*) echo "❌ Unknown service: $$SERVICE_NAME"; echo "Available services: $(GO_SERVICES) $(FRONTEND_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH=$$SERVICE_PATH

# =============================================================================
# BUILD VALIDATION COMMANDS
# =============================================================================

# Validate build for Go services
validate-build-go-service: $(BIN_DIR)
	@echo "Building Go service: $(SERVICE)"
	@cd $(SERVICE_PATH) && go mod tidy
	@cd $(SERVICE_PATH) && CGO_ENABLED=0 go build \
		-ldflags="-w -s -X main.Version=dev -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)" \
		-o $(BIN_DIR)/$(SERVICE) .
	@echo "✅ Go service $(SERVICE) built successfully -> $(BIN_DIR)/$(SERVICE)"

# Validate build for Frontend services
validate-build-frontend-service:
	@echo "Building Frontend service: $(SERVICE)"
	@cd $(SERVICE_PATH) && pnpm install --frozen-lockfile
	@cd $(SERVICE_PATH) && pnpm run build
	@echo "✅ Frontend service $(SERVICE) built successfully -> $(SERVICE_PATH)/dist/"

# Build validation router
validate-build:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; TARGET="validate-build-go-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; TARGET="validate-build-go-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; TARGET="validate-build-go-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; TARGET="validate-build-go-service" ;; \
		member-portal) SERVICE_PATH="$(MEMBER_PORTAL_PATH)"; TARGET="validate-build-frontend-service" ;; \
		admin-portal) SERVICE_PATH="$(ADMIN_PORTAL_PATH)"; TARGET="validate-build-frontend-service" ;; \
		consent-portal) SERVICE_PATH="$(CONSENT_PORTAL_PATH)"; TARGET="validate-build-frontend-service" ;; \
		*) echo "❌ Unknown service: $$SERVICE_NAME"; echo "Available services: $(GO_SERVICES) $(FRONTEND_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH=$$SERVICE_PATH

# =============================================================================
# TEST VALIDATION COMMANDS
# =============================================================================

# Validate tests for Go services
validate-test-go-service:
	@echo "Running tests for Go service: $(SERVICE)"
	@cd $(SERVICE_PATH) && go mod tidy
	@echo "Running unit tests with coverage..."
	@cd $(SERVICE_PATH) && go test -v -race -coverprofile=coverage.out -covermode=atomic ./... || (echo "❌ Tests failed for $(SERVICE)" && exit 1)
	@cd $(SERVICE_PATH) && go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: $(SERVICE_PATH)/coverage.html"
	@cd $(SERVICE_PATH) && go tool cover -func=coverage.out | tail -1
	@echo "✅ Tests passed for Go service $(SERVICE)"

# Validate tests for root-module Go services (implementation lives under
# internal/<svc>, not under the service's own dir, so we can't cd+./...)
validate-test-go-rootmod-service:
	@echo "Running tests for Go service: $(SERVICE)"
	@go mod tidy
	@echo "Running unit tests with coverage..."
	@go test -v -race -coverprofile=coverage-$(SERVICE).out -covermode=atomic $(SERVICE_TEST_PATH) || (echo "❌ Tests failed for $(SERVICE)" && exit 1)
	@go tool cover -html=coverage-$(SERVICE).out -o coverage-$(SERVICE).html
	@echo "Coverage report generated: coverage-$(SERVICE).html"
	@go tool cover -func=coverage-$(SERVICE).out | tail -1
	@echo "✅ Tests passed for Go service $(SERVICE)"

# Validate tests for Frontend services (lint + type check as test equivalent)
validate-test-frontend-service:
	@echo "Running tests for Frontend service: $(SERVICE)"
	@cd $(SERVICE_PATH) && pnpm install --frozen-lockfile
	@echo "Running TypeScript compilation check..."
	@cd $(SERVICE_PATH) && pnpm exec tsc --noEmit || (echo "❌ TypeScript compilation failed for $(SERVICE)" && exit 1)
	@echo "Running lint checks..."
	@cd $(SERVICE_PATH) && pnpm run lint || (echo "❌ Lint checks failed for $(SERVICE)" && exit 1)
	@echo "✅ Tests passed for Frontend service $(SERVICE)"

# Test validation router
validate-test:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; SERVICE_TEST_PATH="$(PORTAL_BACKEND_TEST_PATH)"; TARGET="validate-test-go-rootmod-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; SERVICE_TEST_PATH="$(ORCHESTRATION_ENGINE_TEST_PATH)"; TARGET="validate-test-go-rootmod-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; SERVICE_TEST_PATH="$(CONSENT_ENGINE_TEST_PATH)"; TARGET="validate-test-go-rootmod-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; SERVICE_TEST_PATH="$(POLICY_DECISION_POINT_TEST_PATH)"; TARGET="validate-test-go-rootmod-service" ;; \
		member-portal) SERVICE_PATH="$(MEMBER_PORTAL_PATH)"; TARGET="validate-test-frontend-service" ;; \
		admin-portal) SERVICE_PATH="$(ADMIN_PORTAL_PATH)"; TARGET="validate-test-frontend-service" ;; \
		consent-portal) SERVICE_PATH="$(CONSENT_PORTAL_PATH)"; TARGET="validate-test-frontend-service" ;; \
		*) echo "❌ Unknown service: $$SERVICE_NAME"; echo "Available services: $(GO_SERVICES) $(FRONTEND_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH=$$SERVICE_PATH SERVICE_TEST_PATH="$$SERVICE_TEST_PATH"

# =============================================================================
# DOCKER BUILD VALIDATION COMMANDS
# =============================================================================

# Validate Docker build
validate-docker-build-service:
	@echo "Validating Docker build for service: $(SERVICE)"
	@cd $(SERVICE_PATH) && docker build -t $(SERVICE):test \
		--build-arg BUILD_VERSION=test \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		. || (echo "❌ Docker build failed for $(SERVICE)" && exit 1)
	@echo "✅ Docker build successful for $(SERVICE)"
	@docker rmi $(SERVICE):test 2>/dev/null || true

# Validate Docker build for root-module Go services (build context is the
# repo root; the Dockerfile lives under cmd/<svc>, not the service directory)
validate-docker-build-rootmod-service:
	@echo "Validating Docker build for service: $(SERVICE)"
	@docker build -t $(SERVICE):test \
		--build-arg BUILD_VERSION=test \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-f $(SERVICE_DOCKERFILE) . || (echo "❌ Docker build failed for $(SERVICE)" && exit 1)
	@echo "✅ Docker build successful for $(SERVICE)"
	@docker rmi $(SERVICE):test 2>/dev/null || true

# Docker validation router
validate-docker-build:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_DOCKERFILE="$(PORTAL_BACKEND_DOCKERFILE)"; TARGET="validate-docker-build-rootmod-service" ;; \
		orchestration-engine) SERVICE_DOCKERFILE="$(ORCHESTRATION_ENGINE_DOCKERFILE)"; TARGET="validate-docker-build-rootmod-service" ;; \
		consent-engine) SERVICE_DOCKERFILE="$(CONSENT_ENGINE_DOCKERFILE)"; TARGET="validate-docker-build-rootmod-service" ;; \
		policy-decision-point) SERVICE_DOCKERFILE="$(POLICY_DECISION_POINT_DOCKERFILE)"; TARGET="validate-docker-build-rootmod-service" ;; \
		member-portal) SERVICE_PATH="$(MEMBER_PORTAL_PATH)"; TARGET="validate-docker-build-service" ;; \
		admin-portal) SERVICE_PATH="$(ADMIN_PORTAL_PATH)"; TARGET="validate-docker-build-service" ;; \
		consent-portal) SERVICE_PATH="$(CONSENT_PORTAL_PATH)"; TARGET="validate-docker-build-service" ;; \
		*) echo "❌ Unknown service: $$SERVICE_NAME"; echo "Available services: $(GO_SERVICES) $(FRONTEND_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH=$$SERVICE_PATH SERVICE_DOCKERFILE=$$SERVICE_DOCKERFILE

# =============================================================================
# CODE QUALITY COMMANDS
# =============================================================================

# Install essential Go quality tools (minimal set)
install-tools:
	@echo "Installing essential Go quality tools..."
	@go install mvdan.cc/gofumpt@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "✅ Essential Go quality tools installed"
	@echo "💡 Configure your IDE (VS Code/GoLand) for real-time linting!"
	@echo "ℹ️  For security scanning, install gosec or use 'make security <service>'"

# Format Go code
format-go-service:
	@echo "Formatting Go service: $(SERVICE)"
	@if [ -f "$(SERVICE_PATH)/go.mod" ]; then \
		cd $(SERVICE_PATH) && go mod tidy; \
	else \
		echo "⚠️  No go.mod found in $(SERVICE_PATH), skipping go mod tidy"; \
	fi
	@echo "Running gofumpt..."
	@if command -v $$(go env GOPATH)/bin/gofumpt > /dev/null 2>&1; then \
		cd $(SERVICE_PATH) && $$(go env GOPATH)/bin/gofumpt -w .; \
	else \
		echo "⚠️  gofumpt not available, using gofmt"; \
		cd $(SERVICE_PATH) && gofmt -w .; \
	fi
	@echo "Running goimports..."
	@if command -v $$(go env GOPATH)/bin/goimports > /dev/null 2>&1; then \
		cd $(SERVICE_PATH) && $$(go env GOPATH)/bin/goimports -w .; \
	else \
		echo "⚠️  goimports not available, skipping import organization"; \
	fi
	@echo "✅ Code formatted for Go service $(SERVICE)"

# Format Go code for root-module Go services (implementation split across
# cmd/<svc> and internal/<svc>, so format both instead of the whole SERVICE_PATH)
format-go-rootmod-service:
	@echo "Formatting Go service: $(SERVICE)"
	@go mod tidy
	@echo "Running gofumpt..."
	@if command -v $$(go env GOPATH)/bin/gofumpt > /dev/null 2>&1; then \
		$$(go env GOPATH)/bin/gofumpt -w $(SERVICE_PATH) $(SERVICE_INTERNAL_PATH); \
	else \
		echo "⚠️  gofumpt not available, using gofmt"; \
		gofmt -w $(SERVICE_PATH) $(SERVICE_INTERNAL_PATH); \
	fi
	@echo "Running goimports..."
	@if command -v $$(go env GOPATH)/bin/goimports > /dev/null 2>&1; then \
		$$(go env GOPATH)/bin/goimports -w $(SERVICE_PATH) $(SERVICE_INTERNAL_PATH); \
	else \
		echo "⚠️  goimports not available, skipping import organization"; \
	fi
	@echo "✅ Code formatted for Go service $(SERVICE)"

# Run basic Go linting (using built-in tools)
lint-go-service:
	@echo "Running basic lint checks for Go service: $(SERVICE)"
	@if [ -f "$(SERVICE_PATH)/go.mod" ]; then \
		cd $(SERVICE_PATH) && go mod tidy; \
	else \
		echo "⚠️  No go.mod found in $(SERVICE_PATH), skipping go mod tidy"; \
	fi
	@echo "Running go vet..."
	@cd $(SERVICE_PATH) && go vet ./... || (echo "❌ go vet found issues in $(SERVICE)" && exit 1)
	@echo "Running gofmt check..."
	@cd $(SERVICE_PATH) && test -z "$$(gofmt -l .)" || (echo "❌ Code needs formatting. Run: make format $(SERVICE)" && gofmt -l . && exit 1)
	@echo "✅ Basic lint checks completed for Go service $(SERVICE)"

# Run basic Go linting for root-module Go services
lint-go-rootmod-service:
	@echo "Running basic lint checks for Go service: $(SERVICE)"
	@go mod tidy
	@echo "Running go vet..."
	@go vet $(SERVICE_TEST_PATH) || (echo "❌ go vet found issues in $(SERVICE)" && exit 1)
	@echo "Running gofmt check..."
	@test -z "$$(gofmt -l $(SERVICE_PATH) $(SERVICE_INTERNAL_PATH))" || (echo "❌ Code needs formatting. Run: make format $(SERVICE)" && gofmt -l $(SERVICE_PATH) $(SERVICE_INTERNAL_PATH) && exit 1)
	@echo "✅ Basic lint checks completed for Go service $(SERVICE)"

# Run staticcheck
staticcheck-go-service:
	@echo "Running staticcheck for Go service: $(SERVICE)"
	@if [ -f "$(SERVICE_PATH)/go.mod" ]; then \
		cd $(SERVICE_PATH) && go mod tidy; \
	else \
		echo "⚠️  No go.mod found in $(SERVICE_PATH), skipping go mod tidy"; \
	fi
	@if command -v $$(go env GOPATH)/bin/staticcheck > /dev/null 2>&1; then \
		cd $(SERVICE_PATH) && $$(go env GOPATH)/bin/staticcheck ./... || echo "⚠️  Staticcheck found issues in $(SERVICE) (non-blocking)"; \
	else \
		echo "ℹ️  staticcheck not installed, skipping analysis for $(SERVICE)"; \
	fi
	@echo "✅ Staticcheck completed for Go service $(SERVICE)"

# Run staticcheck for root-module Go services
staticcheck-go-rootmod-service:
	@echo "Running staticcheck for Go service: $(SERVICE)"
	@go mod tidy
	@if command -v $$(go env GOPATH)/bin/staticcheck > /dev/null 2>&1; then \
		$$(go env GOPATH)/bin/staticcheck $(SERVICE_TEST_PATH) || echo "⚠️  Staticcheck found issues in $(SERVICE) (non-blocking)"; \
	else \
		echo "ℹ️  staticcheck not installed, skipping analysis for $(SERVICE)"; \
	fi
	@echo "✅ Staticcheck completed for Go service $(SERVICE)"

# Run security checks with gosec (if available)
security-go-service:
	@echo "Running security checks for Go service: $(SERVICE)"
	@if [ -f "$(SERVICE_PATH)/go.mod" ]; then \
		cd $(SERVICE_PATH) && go mod tidy; \
	else \
		echo "⚠️  No go.mod found in $(SERVICE_PATH), skipping go mod tidy"; \
	fi
	@if command -v gosec > /dev/null 2>&1; then \
		cd $(SERVICE_PATH) && gosec -quiet ./... || echo "⚠️  Security issues found in $(SERVICE) (non-blocking)"; \
	else \
		echo "ℹ️  gosec not installed, skipping security scan for $(SERVICE)"; \
	fi
	@echo "✅ Security check completed for Go service $(SERVICE)"

# Run security checks for root-module Go services
security-go-rootmod-service:
	@echo "Running security checks for Go service: $(SERVICE)"
	@go mod tidy
	@if command -v gosec > /dev/null 2>&1; then \
		gosec -quiet $(SERVICE_TEST_PATH) || echo "⚠️  Security issues found in $(SERVICE) (non-blocking)"; \
	else \
		echo "ℹ️  gosec not installed, skipping security scan for $(SERVICE)"; \
	fi
	@echo "✅ Security check completed for Go service $(SERVICE)"

# Comprehensive quality check for Go services
quality-check-go-service:
	@echo "Running comprehensive quality checks for Go service: $(SERVICE)"
	@$(MAKE) format-go-service SERVICE=$(SERVICE) SERVICE_PATH=$(SERVICE_PATH)
	@$(MAKE) lint-go-service SERVICE=$(SERVICE) SERVICE_PATH=$(SERVICE_PATH)
	@$(MAKE) staticcheck-go-service SERVICE=$(SERVICE) SERVICE_PATH=$(SERVICE_PATH)
	@$(MAKE) security-go-service SERVICE=$(SERVICE) SERVICE_PATH=$(SERVICE_PATH)
	@$(MAKE) validate-test-go-service SERVICE=$(SERVICE) SERVICE_PATH=$(SERVICE_PATH)
	@echo "✅ All quality checks passed for Go service $(SERVICE)"

# Comprehensive quality check for root-module Go services
quality-check-go-rootmod-service:
	@echo "Running comprehensive quality checks for Go service: $(SERVICE)"
	@$(MAKE) format-go-rootmod-service SERVICE=$(SERVICE) SERVICE_PATH=$(SERVICE_PATH) SERVICE_INTERNAL_PATH=$(SERVICE_INTERNAL_PATH)
	@$(MAKE) lint-go-rootmod-service SERVICE=$(SERVICE) SERVICE_PATH=$(SERVICE_PATH) SERVICE_INTERNAL_PATH=$(SERVICE_INTERNAL_PATH) SERVICE_TEST_PATH="$(SERVICE_TEST_PATH)"
	@$(MAKE) staticcheck-go-rootmod-service SERVICE=$(SERVICE) SERVICE_TEST_PATH="$(SERVICE_TEST_PATH)"
	@$(MAKE) security-go-rootmod-service SERVICE=$(SERVICE) SERVICE_TEST_PATH="$(SERVICE_TEST_PATH)"
	@$(MAKE) validate-test-go-rootmod-service SERVICE=$(SERVICE) SERVICE_TEST_PATH="$(SERVICE_TEST_PATH)"
	@echo "✅ All quality checks passed for Go service $(SERVICE)"

# Legacy lint check for Go services (for backward compatibility)
check-lint-go-service:
	@echo "Running basic lint checks for Go service: $(SERVICE)"
	@cd $(SERVICE_PATH) && go mod tidy
	@echo "Running go fmt..."
	@OUTPUT=$$(cd $(SERVICE_PATH) && gofmt -l .); \
	if [ -n "$$OUTPUT" ]; then \
		echo "❌ Files need formatting. Run: make format $(SERVICE)"; \
		echo "$$OUTPUT"; \
		exit 1; \
	fi
	@echo "Running go vet..."
	@cd $(SERVICE_PATH) && go vet ./... || (echo "❌ go vet failed for $(SERVICE)" && exit 1)
	@echo "✅ Basic lint checks completed for Go service $(SERVICE)"

# Legacy lint check for root-module Go services (for backward compatibility)
check-lint-go-rootmod-service:
	@echo "Running basic lint checks for Go service: $(SERVICE)"
	@go mod tidy
	@echo "Running go fmt..."
	@OUTPUT=$$(gofmt -l $(SERVICE_PATH) $(SERVICE_INTERNAL_PATH)); \
	if [ -n "$$OUTPUT" ]; then \
		echo "❌ Files need formatting. Run: make format $(SERVICE)"; \
		echo "$$OUTPUT"; \
		exit 1; \
	fi
	@echo "Running go vet..."
	@go vet $(SERVICE_TEST_PATH) || (echo "❌ go vet failed for $(SERVICE)" && exit 1)
	@echo "✅ Basic lint checks completed for Go service $(SERVICE)"

# Frontend service quality checks
check-lint-frontend-service:
	@echo "Running lint checks for Frontend service: $(SERVICE)"
	@cd $(SERVICE_PATH) && pnpm install --frozen-lockfile
	@cd $(SERVICE_PATH) && pnpm run lint || (echo "❌ Lint checks failed for $(SERVICE)" && exit 1)
	@echo "✅ Lint checks passed for Frontend service $(SERVICE)"

# =============================================================================
# QUALITY CHECK ROUTERS
# =============================================================================

# Format router
format:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; SERVICE_INTERNAL_PATH="$(PORTAL_BACKEND_INTERNAL_PATH)"; TARGET="format-go-rootmod-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; SERVICE_INTERNAL_PATH="$(ORCHESTRATION_ENGINE_INTERNAL_PATH)"; TARGET="format-go-rootmod-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; SERVICE_INTERNAL_PATH="$(CONSENT_ENGINE_INTERNAL_PATH)"; TARGET="format-go-rootmod-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; SERVICE_INTERNAL_PATH="$(POLICY_DECISION_POINT_INTERNAL_PATH)"; TARGET="format-go-rootmod-service" ;; \
		*) echo "❌ Unknown Go service: $$SERVICE_NAME"; echo "Available Go services: $(GO_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH="$$SERVICE_PATH" SERVICE_INTERNAL_PATH="$$SERVICE_INTERNAL_PATH"

# Lint router
lint:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; SERVICE_INTERNAL_PATH="$(PORTAL_BACKEND_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(PORTAL_BACKEND_TEST_PATH)"; TARGET="lint-go-rootmod-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; SERVICE_INTERNAL_PATH="$(ORCHESTRATION_ENGINE_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(ORCHESTRATION_ENGINE_TEST_PATH)"; TARGET="lint-go-rootmod-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; SERVICE_INTERNAL_PATH="$(CONSENT_ENGINE_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(CONSENT_ENGINE_TEST_PATH)"; TARGET="lint-go-rootmod-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; SERVICE_INTERNAL_PATH="$(POLICY_DECISION_POINT_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(POLICY_DECISION_POINT_TEST_PATH)"; TARGET="lint-go-rootmod-service" ;; \
		*) echo "❌ Unknown Go service: $$SERVICE_NAME"; echo "Available Go services: $(GO_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH="$$SERVICE_PATH" SERVICE_INTERNAL_PATH="$$SERVICE_INTERNAL_PATH" SERVICE_TEST_PATH="$$SERVICE_TEST_PATH"

# Staticcheck router
staticcheck:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; SERVICE_TEST_PATH="$(PORTAL_BACKEND_TEST_PATH)"; TARGET="staticcheck-go-rootmod-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; SERVICE_TEST_PATH="$(ORCHESTRATION_ENGINE_TEST_PATH)"; TARGET="staticcheck-go-rootmod-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; SERVICE_TEST_PATH="$(CONSENT_ENGINE_TEST_PATH)"; TARGET="staticcheck-go-rootmod-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; SERVICE_TEST_PATH="$(POLICY_DECISION_POINT_TEST_PATH)"; TARGET="staticcheck-go-rootmod-service" ;; \
		*) echo "❌ Unknown Go service: $$SERVICE_NAME"; echo "Available Go services: $(GO_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH="$$SERVICE_PATH" SERVICE_TEST_PATH="$$SERVICE_TEST_PATH"

# Security router
security:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; SERVICE_TEST_PATH="$(PORTAL_BACKEND_TEST_PATH)"; TARGET="security-go-rootmod-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; SERVICE_TEST_PATH="$(ORCHESTRATION_ENGINE_TEST_PATH)"; TARGET="security-go-rootmod-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; SERVICE_TEST_PATH="$(CONSENT_ENGINE_TEST_PATH)"; TARGET="security-go-rootmod-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; SERVICE_TEST_PATH="$(POLICY_DECISION_POINT_TEST_PATH)"; TARGET="security-go-rootmod-service" ;; \
		*) echo "❌ Unknown Go service: $$SERVICE_NAME"; echo "Available Go services: $(GO_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH="$$SERVICE_PATH" SERVICE_TEST_PATH="$$SERVICE_TEST_PATH"

# Quality check router
quality-check:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; SERVICE_INTERNAL_PATH="$(PORTAL_BACKEND_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(PORTAL_BACKEND_TEST_PATH)"; TARGET="quality-check-go-rootmod-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; SERVICE_INTERNAL_PATH="$(ORCHESTRATION_ENGINE_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(ORCHESTRATION_ENGINE_TEST_PATH)"; TARGET="quality-check-go-rootmod-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; SERVICE_INTERNAL_PATH="$(CONSENT_ENGINE_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(CONSENT_ENGINE_TEST_PATH)"; TARGET="quality-check-go-rootmod-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; SERVICE_INTERNAL_PATH="$(POLICY_DECISION_POINT_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(POLICY_DECISION_POINT_TEST_PATH)"; TARGET="quality-check-go-rootmod-service" ;; \
		member-portal) SERVICE_PATH="$(MEMBER_PORTAL_PATH)"; TARGET="check-lint-frontend-service" ;; \
		admin-portal) SERVICE_PATH="$(ADMIN_PORTAL_PATH)"; TARGET="check-lint-frontend-service" ;; \
		consent-portal) SERVICE_PATH="$(CONSENT_PORTAL_PATH)"; TARGET="check-lint-frontend-service" ;; \
		*) echo "❌ Unknown service: $$SERVICE_NAME"; echo "Available services: $(GO_SERVICES) $(FRONTEND_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH="$$SERVICE_PATH" SERVICE_INTERNAL_PATH="$$SERVICE_INTERNAL_PATH" SERVICE_TEST_PATH="$$SERVICE_TEST_PATH"

# Legacy lint check router (for backward compatibility)
check-lint:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; SERVICE_INTERNAL_PATH="$(PORTAL_BACKEND_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(PORTAL_BACKEND_TEST_PATH)"; TARGET="check-lint-go-rootmod-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; SERVICE_INTERNAL_PATH="$(ORCHESTRATION_ENGINE_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(ORCHESTRATION_ENGINE_TEST_PATH)"; TARGET="check-lint-go-rootmod-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; SERVICE_INTERNAL_PATH="$(CONSENT_ENGINE_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(CONSENT_ENGINE_TEST_PATH)"; TARGET="check-lint-go-rootmod-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; SERVICE_INTERNAL_PATH="$(POLICY_DECISION_POINT_INTERNAL_PATH)"; SERVICE_TEST_PATH="$(POLICY_DECISION_POINT_TEST_PATH)"; TARGET="check-lint-go-rootmod-service" ;; \
		member-portal) SERVICE_PATH="$(MEMBER_PORTAL_PATH)"; TARGET="check-lint-frontend-service" ;; \
		admin-portal) SERVICE_PATH="$(ADMIN_PORTAL_PATH)"; TARGET="check-lint-frontend-service" ;; \
		consent-portal) SERVICE_PATH="$(CONSENT_PORTAL_PATH)"; TARGET="check-lint-frontend-service" ;; \
		*) echo "❌ Unknown service: $$SERVICE_NAME"; echo "Available services: $(GO_SERVICES) $(FRONTEND_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH="$$SERVICE_PATH" SERVICE_INTERNAL_PATH="$$SERVICE_INTERNAL_PATH" SERVICE_TEST_PATH="$$SERVICE_TEST_PATH"

# =============================================================================
# RUN COMMANDS
# =============================================================================

# Run Go services
run-go-service:
	@echo "Running Go service: $(SERVICE)"
	@echo "Service will run in foreground. Press Ctrl+C to stop."
	@cd $(SERVICE_PATH) && go run .

# Run Frontend services
run-frontend-service:
	@echo "Running Frontend service: $(SERVICE)"
	@echo "Service will run in foreground. Press Ctrl+C to stop."
	@cd $(SERVICE_PATH) && pnpm run dev

# Run router
run:
	@SERVICE_NAME="$(word 2,$(MAKECMDGOALS))"; \
	case "$$SERVICE_NAME" in \
		portal-backend) SERVICE_PATH="$(portal_backend_PATH)"; TARGET="run-go-service" ;; \
		orchestration-engine) SERVICE_PATH="$(ORCHESTRATION_ENGINE_PATH)"; TARGET="run-go-service" ;; \
		consent-engine) SERVICE_PATH="$(CONSENT_ENGINE_PATH)"; TARGET="run-go-service" ;; \
		policy-decision-point) SERVICE_PATH="$(POLICY_DECISION_POINT_PATH)"; TARGET="run-go-service" ;; \
		member-portal) SERVICE_PATH="$(MEMBER_PORTAL_PATH)"; TARGET="run-frontend-service" ;; \
		admin-portal) SERVICE_PATH="$(ADMIN_PORTAL_PATH)"; TARGET="run-frontend-service" ;; \
		consent-portal) SERVICE_PATH="$(CONSENT_PORTAL_PATH)"; TARGET="run-frontend-service" ;; \
		*) echo "❌ Unknown service: $$SERVICE_NAME"; echo "Available services: $(GO_SERVICES) $(FRONTEND_SERVICES)"; exit 1 ;; \
	esac; \
	$(MAKE) $$TARGET SERVICE=$$SERVICE_NAME SERVICE_PATH=$$SERVICE_PATH

# =============================================================================
# UTILITY COMMANDS
# =============================================================================

# Clean all build artifacts
clean:
	@echo "Cleaning all build artifacts..."
	@rm -rf $(BIN_DIR)
	@find . -name "coverage*.out" -delete 2>/dev/null || true
	@find . -name "coverage*.html" -delete 2>/dev/null || true
	@find . -type d -name "node_modules" | while read dir; do rm -rf "$$dir"; done 2>/dev/null || true
	@rm -rf portals/apps/member/dist portals/apps/admin/dist portals/apps/consent/dist 2>/dev/null || true
	@echo "✅ All build artifacts cleaned"

# Allow service names to be used as targets (ignore them)
$(GO_SERVICES) $(FRONTEND_SERVICES):
	@:

# =============================================================================
# BATCH OPERATIONS
# =============================================================================

# Setup all services
setup-all:
	@echo "Setting up all services..."
	@$(MAKE) install-hooks
	@for service in $(GO_SERVICES); do \
		echo "Setting up $$service..."; \
		$(MAKE) setup $$service; \
	done
	@for service in $(FRONTEND_SERVICES); do \
		echo "Setting up $$service..."; \
		$(MAKE) setup $$service; \
	done
	@echo "✅ All services setup complete"

# Build all services
validate-build-all:
	@echo "Building all services..."
	@for service in $(GO_SERVICES); do \
		echo "Building $$service..."; \
		$(MAKE) validate-build $$service; \
	done
	@for service in $(FRONTEND_SERVICES); do \
		echo "Building $$service..."; \
		$(MAKE) validate-build $$service; \
	done
	@echo "✅ All services built successfully"

# Test all services
validate-test-all:
	@echo "Testing all services..."
	@for service in $(GO_SERVICES); do \
		echo "Testing $$service..."; \
		$(MAKE) validate-test $$service; \
	done
	@for service in $(FRONTEND_SERVICES); do \
		echo "Testing $$service..."; \
		$(MAKE) validate-test $$service; \
	done
	@echo "✅ All services tested successfully"

# Quality check all Go services
quality-check-all:
	@echo "Running quality checks on all Go services..."
	@set -e; \
	for service in $(GO_SERVICES); do \
		echo "Quality checking $$service..."; \
		$(MAKE) quality-check $$service & \
	done; \
	wait

# Format all Go services
format-all:
	@echo "Formatting all Go services..."
	@for service in $(GO_SERVICES); do \
		echo "Formatting $$service..."; \
		$(MAKE) format $$service; \
	done
	@echo "✅ All Go services formatted"

# Lint all Go services
lint-all:
	@echo "Linting all Go services..."
	@for service in $(GO_SERVICES); do \
		echo "Linting $$service..."; \
		$(MAKE) lint $$service; \
	done
	@echo "✅ All Go services passed lint checks"

