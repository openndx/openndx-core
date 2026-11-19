# OpenNDX developer commands. Run `make help` for the list.
#
# Every target works on the whole Go module; narrow `test` with PKG, e.g.
#   make test PKG=./internal/pdp/...
# CI runs the same tools with the same settings (.golangci.yml, tools/go.mod).

# Recipes are POSIX sh. On Windows, make would otherwise fall back to cmd.exe
# when no sh.exe is on PATH, so point it at the sh and coreutils that ship
# with Git for Windows. `git --exec-path` is <git>/<mingw64|clangarm64|...>/
# libexec/git-core, and the tools are in <git>/usr/bin.
ifeq ($(OS),Windows_NT)
GIT_USR_BIN := $(subst /libexec/git-core,/../usr/bin,$(shell git --exec-path))
export PATH := $(GIT_USR_BIN);$(PATH)
SHELL := sh.exe
endif

# golangci-lint, pinned in tools/go.mod so its dependencies stay out of the
# service module. The first run compiles it; later runs use the build cache.
GOLANGCI_LINT := go tool -modfile=tools/go.mod golangci-lint

# Packages for `make test`.
PKG ?= ./...

# Extra golangci-lint flags for `make lint`, e.g. --new-from-rev=HEAD to
# report only issues in uncommitted changes, or --new-from-merge-base=main
# for the whole branch.
LINT_FLAGS ?=

# Services for `make dev`: any of pdp ce oe pb portal. Empty runs all of them.
SVC ?=

# Containers `make deps` starts: everything the services need, not the
# services themselves. Compose also starts what these depend on (the
# ThunderID init and setup steps).
DEPS := postgres thunderid audit-service

# Mock data providers OE federates to (see dev/README.md).
PROVIDERS := docker compose -f dev/compose.yml

.DEFAULT_GOAL := help
.PHONY: help setup build test test-integration fmt lint check deps up down dev clean

help: ## List the targets
	@awk 'BEGIN { FS = ":.*## " } /^[a-z-]+:.*## / { printf "  %-17s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

setup: ## Install the git hook, Go modules, golangci-lint and portal packages
	git config core.hooksPath .githooks
	go mod download
	$(GOLANGCI_LINT) version
	cd portals && pnpm install --frozen-lockfile --filter consent-portal...

build: ## Build every package
	go build ./...

test: ## Run unit tests with the race detector (PKG=./internal/pdp/... for one package)
	go test -race $(PKG)

test-integration: ## Run the integration tests against services in Docker
	bash tests/integration/run-local-tests.sh

fmt: ## Format Go code (gofumpt, goimports)
	$(GOLANGCI_LINT) fmt

lint: ## Lint Go code (LINT_FLAGS=--new-from-rev=HEAD for only your uncommitted changes)
	$(GOLANGCI_LINT) run $(LINT_FLAGS) ./...

check: ## Check go.mod is tidy, lint and test, changing nothing; run by the pre-commit hook
	go mod tidy -diff
	$(MAKE) --no-print-directory lint
	$(MAKE) --no-print-directory test

deps: ## Start only the dependencies in Docker, to run the services with `make dev`
	docker compose up -d $(DEPS)
	$(PROVIDERS) up -d

up: ## Build and start the full stack in Docker
	docker compose up -d --build
	$(PROVIDERS) up -d

down: ## Stop everything `deps` or `up` started
	$(PROVIDERS) down
	docker compose down

dev: ## Run the services natively with labelled logs (SVC="oe pb" for some)
	go run ./dev/run $(SVC)

clean: ## Remove bin/ and coverage files
	rm -rf bin
	find . -name node_modules -prune -o -type f \( -name 'coverage*.out' -o -name 'coverage*.html' \) -exec rm -f {} +
