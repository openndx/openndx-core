# Development Guide

This guide will help you set up your local development environment for contributing to OpenNDX Core.

## Prerequisites

Before you begin, ensure you have the following installed:

-   [Go](https://go.dev/dl/) 1.26 or later
-   [Node](https://nodejs.org/) 24 or later with [pnpm](https://pnpm.io/) (for the Consent Portal)
-   [Docker](https://www.docker.com/products/docker-desktop/) with Docker Compose
-   [Make](https://www.gnu.org/software/make/)
-   [Git](https://git-scm.com/downloads)

On Windows, install GNU Make (for example `choco install make`) and Git for Windows. The Makefile runs its recipes with the `sh` that ships with Git, so `make` works from PowerShell, cmd or Git Bash.

`make test`, `make check` and so the pre-commit hook use the race detector, which needs a C compiler on Linux (gcc, for example from `build-essential`) and Windows. macOS needs none.

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

3.  **Run the setup:**
    ```bash
    make setup
    ```

    This points Git at the pre-commit hook in `.githooks`, downloads the Go modules, builds the pinned golangci-lint, and installs the Consent Portal's packages. Run it again after pulling changes to `tools/go.mod` or the portal's dependencies.

## Make targets

`make help` lists them. Every Go target works on the whole module.

| Target | Does |
|---|---|
| `make setup` | Installs the git hook, Go modules, golangci-lint and portal packages |
| `make build` | `go build ./...` |
| `make test` | `go test -race ./...`; `make test PKG=./internal/pdp/...` for some packages |
| `make test-integration` | Runs the integration tests against services in Docker (see [tests/integration](../../tests/integration/README.md)) |
| `make fmt` | Formats Go code with gofumpt and goimports |
| `make lint` | Runs golangci-lint with [.golangci.yml](../../.golangci.yml): vet, staticcheck, gosec and the rest. `LINT_FLAGS` passes extra flags |
| `make check` | Checks `go.mod` is tidy, then `lint` and `test`; changes nothing |
| `make deps` | Starts only the dependencies in Docker, to run the services natively |
| `make up` / `make down` | Starts or stops the full stack in Docker |
| `make clean` | Removes `bin/` and coverage files |

golangci-lint is pinned in [tools/go.mod](../../tools/go.mod) and runs through `go tool`, so there is nothing to install; the first run compiles it. The CI validate workflows read the version from the same file.

The pre-commit hook runs `make check` when a commit touches Go code. `make check` fails while existing lint issues remain ([#791](https://github.com/openndx/openndx-core/issues/791)); until they are fixed, commit with `git commit --no-verify`, and see what your change adds with:

```bash
make lint LINT_FLAGS=--new-from-rev=HEAD           # uncommitted changes
make lint LINT_FLAGS=--new-from-merge-base=main    # everything on your branch
```

## Running the platform

A single service rarely runs on its own: the Orchestration Engine needs the PDP, the Consent Engine, ThunderID and data providers, and the Portal Backend needs PostgreSQL, ThunderID and the PDP. There are two supported ways to run the platform.

### Full stack in Docker

```bash
make up      # build and start everything, plus the mock data providers
make down    # stop it
```

This is the default for trying the platform; the [end-to-end walkthrough](../../dev/WALKTHROUGH.md) uses it. A bare `docker compose up --build` still starts the same stack without the mock providers.

### Dependencies in Docker, services native

The development loop: fast rebuilds, a debugger, plain logs.

```bash
make deps    # PostgreSQL (with every schema.sql loaded), ThunderID, the audit service, the mock providers
```

Then run each service in a terminal of its own, from its directory, where it reads `config.yaml`:

```bash
cd cmd/pdp && go run .
cd cmd/ce && go run .
cd cmd/oe && go run .
cd cmd/pb && go run .
cd portals/apps/consent && pnpm dev    # reads public/config.js
```

Before the first run, create the git-ignored config files from the examples next to them:

| Copy | To | Then |
|---|---|---|
| `cmd/pdp/config.example.yaml` | `cmd/pdp/config.yaml` | |
| `cmd/ce/config.example.yaml` | `cmd/ce/config.yaml` | |
| `cmd/oe/config.example.yaml` | `cmd/oe/config.yaml` | |
| `cmd/pb/config.example.yaml` | `cmd/pb/config.yaml` | |
| `portals/apps/consent/public/config.example.js` | `portals/apps/consent/public/config.js` | Set `idpClientId: 'CONSENT_PORTAL'` and `idpBaseUrl: 'http://localhost:8090'` |

The example configs point at the ports `make deps` publishes on `localhost` and read their secrets from the environment. With the Compose defaults:

```bash
export DB_PASSWORD=1234          # PowerShell: $env:DB_PASSWORD = "1234"
export IDP_CLIENT_SECRET=1234    # Portal Backend's ThunderID client secret
```

If something else on your machine already uses port 5432 (a local PostgreSQL install, for example), set `DB_PORT` in `.env` to a free port before `make deps`, and use that port in the configs.

### One service native, the rest in Docker

Native services reach containers through ports on `localhost`, but containers reach each other by service name, never the host. So this works only for a service no container calls: the Orchestration Engine and the Portal Backend.

```bash
make up
docker compose stop orchestration-engine
cd cmd/oe && go run .
```

`make up` starts every service in Docker, on the same host ports the native services use. Stop any service you run natively before running it, or Docker can't publish that port and `make up` stops before starting the mock providers.

Running the PDP or the Consent Engine natively means running their callers natively too, which is the mode above.

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

### Before you push

```bash
make fmt
make check
```

Run `make test-integration` too when your change affects how services talk to each other.

## Code Style and Standards

### Go Code Style

-   Follow standard Go idioms and conventions
-   Format with `make fmt` and lint with `make lint`; both use [.golangci.yml](../../.golangci.yml)

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
│   └── pb/                # Portal Backend
├── internal/              # Go service internals
│   ├── oe/
│   ├── ce/
│   ├── pdp/
│   └── pb/
├── dev/                   # Mock data providers
├── portals/               # Frontend React applications (pnpm workspace)
│   ├── apps/
│   │   ├── member/
│   │   ├── admin/
│   │   └── consent/
│   └── packages/          # Shared code used across the portals
├── tests/                 # Integration tests
│   └── integration/
├── tools/                 # Pinned developer tools (golangci-lint)
└── docs/                  # Documentation
```

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
