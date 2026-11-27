# OpenNDX developer commands. Run `make help` for the list.
#
# Every target works on the whole Go module; narrow `test` with PKG, e.g.
#   make test PKG=./internal/pdp/...
# golangci-lint uses .golangci.yml and GOLANGCI_LINT_VERSION below, here and
# in the CI validate workflows, which read the version from this file.

# Recipes are POSIX sh. On Windows, make would otherwise fall back to cmd.exe
# when no sh.exe is on PATH, so point it at the sh and coreutils that ship
# with Git for Windows. `git --exec-path` is <git>/<mingw64|clangarm64|...>/
# libexec/git-core, and the tools are in <git>/usr/bin.
ifeq ($(OS),Windows_NT)
GIT_USR_BIN := $(subst /libexec/git-core,/../usr/bin,$(shell git --exec-path))
export PATH := $(GIT_USR_BIN);$(PATH)
SHELL := sh.exe
EXE := .exe
endif

# golangci-lint's release binary, the one CI runs. The first `make setup`,
# `fmt` or `lint` downloads it with the installer from the same release tag,
# which checks it against the release's SHA-256 checksums. The version is in
# the path, so a bump downloads the new release.
GOLANGCI_LINT_VERSION := v2.14.0
GOLANGCI_LINT := bin/golangci-lint-$(GOLANGCI_LINT_VERSION)/golangci-lint$(EXE)
GOLANGCI_LINT_INSTALLER := https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh

# Packages for `make test`.
PKG ?= ./...

# Extra golangci-lint flags for `make lint`, e.g. --new-from-rev=HEAD to
# report only issues in uncommitted changes, or --new-from-merge-base=main
# for the whole branch.
LINT_FLAGS ?=

# Containers `make deps` starts: everything the services need, not the
# services themselves. Compose also starts what these depend on (the
# ThunderID init and setup steps).
DEPS := postgres thunderid

# OTEL=1 with `make deps` or `make up` also starts a local OpenObserve on
# http://localhost:5080 (Compose profile `otel`) to receive the Orchestration
# Engine's and Portal Backend's audit records. They send records to it only
# once the otel block in their config is uncommented; see
# CONTRIBUTING.md#viewing-audit-records.
OTEL ?=
ifeq ($(OTEL),1)
DEPS += openobserve
COMPOSE_PROFILE := --profile otel
endif

# Mock data providers OE federates to (see dev/README.md).
PROVIDERS := docker compose -f dev/compose.yml

# Docs checks, the same as the Docs CI workflow (.github/workflows/docs-ci.yml).
# markdownlint and Prettier run through npx (Node.js); lychee and Vale run in
# Docker. The pre-commit hook pins the same markdownlint-cli2 version.
MDLINT_VERSION := 0.23.2
# markdownlint --fix cannot pad tables, so mdlint-fix runs Prettier for them.
PRETTIER_VERSION := 3.9.9
LYCHEE_IMAGE := lycheeverse/lychee:0.24.2
VALE_IMAGE := jdkato/vale:v3.24.0
# Every tracked Markdown file except the paused Admin and Member portals'.
# Lazy (=), so git runs only for a target that uses the list.
MD_FILES = $(shell git ls-files "*.md" ":!portals/apps/admin" ":!portals/apps/member")

.DEFAULT_GOAL := help
.PHONY: help setup build test test-integration fmt lint license license-check check mdlint mdlint-fix linkcheck prose docs-check deps up down clean

help: ## List the targets
	@awk 'BEGIN { FS = ":.*## " } /^[a-z-]+:.*## / { printf "  %-17s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

setup: $(GOLANGCI_LINT) ## Install the git hook, Go modules, golangci-lint and portal packages
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

fmt: $(GOLANGCI_LINT) ## Format Go code (gofumpt, goimports)
	$(GOLANGCI_LINT) fmt

lint: $(GOLANGCI_LINT) ## Lint Go code (LINT_FLAGS=--new-from-rev=HEAD for only your uncommitted changes)
	$(GOLANGCI_LINT) run $(LINT_FLAGS) ./...

# scripts/license-headers.sh decides which files need the SPDX header
# (.github/license-header.tmpl) and lists the ones left out.
license: ## Add the SPDX license header to files that lack it
	bash scripts/license-headers.sh add

license-check: ## List files without the license header, and fail if any, as CI does
	bash scripts/license-headers.sh check

check: ## Check go.mod is tidy and the license headers, lint and test, changing nothing; run by the pre-commit hook
	go mod tidy -diff
	$(MAKE) --no-print-directory license-check
	$(MAKE) --no-print-directory lint LINT_FLAGS=
	$(MAKE) --no-print-directory test PKG=./...

mdlint: ## Lint the Markdown files (rules: .markdownlint-cli2.jsonc)
	npx --yes markdownlint-cli2@$(MDLINT_VERSION) --no-globs $(MD_FILES)

mdlint-fix: ## Apply markdownlint's fixes and align the tables with Prettier
	npx --yes markdownlint-cli2@$(MDLINT_VERSION) --fix --no-globs $(MD_FILES) || true
	npx --yes prettier@$(PRETTIER_VERSION) --prose-wrap preserve --embedded-language-formatting off --write $(MD_FILES)
	npx --yes markdownlint-cli2@$(MDLINT_VERSION) --no-globs $(MD_FILES)

linkcheck: ## Check the links and #anchors in the Markdown files with lychee (needs Docker)
	docker run --rm -e GITHUB_TOKEN -v "$(CURDIR):/input" -w /input $(LYCHEE_IMAGE) --no-progress $(MD_FILES)

prose: ## Check spelling and term casing in the Markdown files with Vale (needs Docker)
	docker run --rm -v "$(CURDIR):/docs" -w /docs $(VALE_IMAGE) $(MD_FILES)

docs-check: mdlint linkcheck prose ## Run the docs checks: mdlint, linkcheck, prose

deps: ## Start only the dependencies in Docker, to run the services natively (OTEL=1 adds OpenObserve)
	docker compose $(COMPOSE_PROFILE) up -d $(DEPS)
	$(PROVIDERS) up -d

up: ## Build and start the full stack in Docker (OTEL=1 adds OpenObserve)
	docker compose $(COMPOSE_PROFILE) up -d --build
	$(PROVIDERS) up -d

down: ## Stop everything `deps` or `up` started
	$(PROVIDERS) down
	docker compose --profile otel down

clean: ## Remove bin/ and coverage files
	rm -rf bin
	find . -name node_modules -prune -o -type f \( -name 'coverage*.out' -o -name 'coverage*.html' \) -exec rm -f {} +

# Saved to a file first: piped into sh, a failed download would still succeed.
$(GOLANGCI_LINT):
	mkdir -p $(@D)
	curl -sSfL -o $(@D)/install.sh $(GOLANGCI_LINT_INSTALLER)
	sh $(@D)/install.sh -b $(@D) $(GOLANGCI_LINT_VERSION)
	rm $(@D)/install.sh
