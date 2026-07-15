# Development Guide

This guide will help you set up your local development environment for contributing to OpenDIF Core.

## Prerequisites

Before you begin, ensure you have the following installed:

-   [Go](https://go.dev/dl/) (1.21 or later)
-   [Docker](https://www.docker.com/products/docker-desktop/) and Docker Compose
-   [Make](https://www.gnu.org/software/make/)
-   [Git](https://git-scm.com/downloads)

## Initial Setup

1.  **Fork and clone the repository:**
    ```bash
    git clone https://github.com/YOUR_USERNAME/opendif-mvp.git
    cd opendif-mvp
    ```

2.  **Add the upstream remote:**
    ```bash
    git remote add upstream https://github.com/OpenDIF/opendif-mvp.git
    ```

3.  **Run the setup script:**
    ```bash
    make setup-all
    ```
    
    This will:
    - Install Git hooks (pre-commit checks)
    - Install Go dependencies for all services
    - Install npm dependencies for all frontend portals

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
cd exchange/consent-engine
go test ./...
```

**Integration Tests:**
```bash
cd tests/integration
docker compose -f docker-compose.test.yml up -d
go test -v ./...
docker compose -f docker-compose.test.yml down
```

**Build Validation:**
```bash
# Build all services
make validate-build-all

# Build a specific service
cd exchange/consent-engine
go build .
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
opendif-mvp/
├── exchange/              # Go backend services
│   ├── orchestration-engine/
│   ├── policy-decision-point/
│   ├── consent-engine/
│   └── ...
├── portals/               # Frontend React applications
│   ├── member-portal/
│   ├── admin-portal/
│   └── consent-portal/
├── tests/                 # Integration tests
│   └── integration/
└── docs/                  # Documentation
```

## Getting Help

-   Check existing [Issues](https://github.com/OpenDIF/opendif-mvp/issues)
-   Review [Pull Request Guidelines](pull-requests.md)
-   See [Reporting Issues](reporting-issues.md) for bug reports

## Next Steps

Once your development environment is set up:

1.   Find an issue to work on or create a new one
2.   Create a branch for your changes
3.   Make your changes and test them
4.   Submit a pull request following our [Pull Request Guidelines](pull-requests.md)
