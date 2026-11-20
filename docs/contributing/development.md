# Development Guide

This guide will help you set up your local development environment for contributing to OpenNDX Core.

## Prerequisites

Before you begin, ensure you have the following installed:

-   [Go](https://go.dev/dl/) 1.26 or later (`go.mod` declares 1.26.4)
-   [Node.js](https://nodejs.org/) 24 or later and [pnpm](https://pnpm.io/), for the Consent Portal
-   [Docker](https://www.docker.com/products/docker-desktop/) and Docker Compose
-   [Make](https://www.gnu.org/software/make/)
-   [Git](https://git-scm.com/downloads)

## Initial Setup

1.  **Fork and clone the repository:**
    ```bash
    git clone https://github.com/YOUR_USERNAME/openndx-core.git
    cd openndx-core
    ```

2.  **Add the upstream remote:**
    ```bash
    git remote add upstream https://github.com/OpenNDX/openndx-core.git
    ```

3.  **Run the setup script:**
    ```bash
    make setup-all
    ```
    
    This will:
    - Install Git hooks (pre-commit checks)
    - Install Go dependencies for all services
    - Install pnpm dependencies for the Consent Portal

## Development Workflow

### Creating a Branch

Always create a new branch from `main` for your changes:

```bash
git checkout main
git pull upstream main
git checkout -b feature/your-feature-name
# or
git checkout -b fix/issue-number
```

### Running Tests

**Unit Tests:**
```bash
# Run all unit tests
go test ./...

# Run tests for a specific service
go test ./cmd/ce/... ./internal/ce/...
```

**Integration Tests:**
```bash
cd tests/integration
docker compose -f docker-compose.db.yml -f docker-compose.test.yml up -d --build
go test -v ./...
docker compose -f docker-compose.db.yml -f docker-compose.test.yml down -v
```

`docker-compose.test.yml` alone is not a valid project: the services depend on databases defined in `docker-compose.db.yml`. `./run-local-tests.sh` runs this same pair and then the tests.

**Build Validation:**
```bash
# Build all services
make validate-build-all

# Build a specific service
go build ./cmd/ce
```

## Code Style and Standards

### Go Code Style

-   Follow standard Go idioms and conventions
-   Run `go fmt ./...` before committing
-   Run `go vet ./...` to catch common mistakes
-   Use `golangci-lint` if available (optional but recommended)

### Commit Messages

Write clear, descriptive commit messages:

-   Use the imperative mood ("Add feature" not "Added feature")
-   Keep the first line under 50 characters
-   Add a blank line and detailed explanation if needed
-   Reference issues: `Fixes #123` or `Closes #456`

Example:
```
Fix consent-engine database connection handling

- Handle connection timeouts gracefully
- Add retry logic for transient failures
- Update error messages for better debugging

Fixes #123
```

### Code Review Checklist

Before submitting a pull request, ensure:

-   [ ] Code follows project style guidelines
-   [ ] All tests pass locally
-   [ ] New code includes appropriate tests
-   [ ] Documentation is updated if needed
-   [ ] Commit messages are clear and descriptive
-   [ ] No merge conflicts with `main` branch

## Project Structure

```
openndx-core/
├── cmd/                   # Go service entrypoints
│   ├── oe/                # Orchestration Engine
│   ├── ce/                # Consent Engine
│   ├── pdp/               # Policy Decision Point
│   ├── pb/                # Portal Backend
│   └── ondx/              # Operator CLI
├── internal/              # Go service internals
│   ├── oe/
│   ├── ce/
│   ├── pdp/
│   └── pb/
├── portals/               # Frontend React applications (pnpm workspace)
│   ├── apps/
│   │   ├── member/
│   │   ├── admin/
│   │   └── consent/
│   └── packages/          # Shared code used across the portals
├── tests/                 # Integration tests
│   └── integration/
└── docs/                  # Documentation
```

## Upstream and downstream

A patch goes upstream as a pull request to `main` on [openndx/openndx-core](https://github.com/openndx/openndx-core). Follow the [pull request guide](pull-requests.md).

A downstream copy is your fork, run with [Deploy](../DEPLOY.md). `docker compose up --build` builds from that checkout, so commits on the fork are what starts. Take newer upstream work with:

```bash
git fetch upstream
git checkout main
git merge upstream/main
docker compose up --build
```

Images and `ondx` binaries published by the project are built from upstream tags. Rebuild from the fork to run a downstream patch. Keep deployment-only settings (`.env`, `cmd/<svc>/config.yaml`) out of the pull request.

## Getting Help

-   Check existing [Issues](https://github.com/OpenNDX/openndx-core/issues)
-   Review [Pull Request Guidelines](pull-requests.md)
-   See [Reporting Issues](reporting-issues.md) for bug reports

## Next Steps

Once your development environment is set up:

1.   Find an issue to work on or create a new one
2.   Create a branch for your changes
3.   Make your changes and test them
4.   Submit a pull request following our [Pull Request Guidelines](pull-requests.md)
