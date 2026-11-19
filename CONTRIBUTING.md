# Contributing to OpenNDX Core

Thank you for your interest in contributing to the OpenNDX Core project! We welcome contributions from everyone and are grateful for your help in making this project better.

## Code of Conduct

This project adheres to a [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Prerequisites

| Tool | Version | Used for |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.26.4 or later, as set in [`go.mod`](go.mod) | every service and the `ondx` CLI |
| Git, Make and a POSIX shell | any recent | the `make` commands and the pre-commit hook; on Windows, use WSL |
| [Node.js](https://nodejs.org/) and [pnpm](https://pnpm.io/installation) | Node 24 or later; pnpm 11, pinned in [`portals/package.json`](portals/package.json) | the Consent Portal, and `make setup-all`, which installs its dependencies |
| [Docker](https://docs.docker.com/get-docker/) with Compose v2 | any recent | the full local stack and the integration tests; unit tests don't need it |
| [golangci-lint](https://golangci-lint.run/welcome/install/) | v2.14.0, the version CI pins | the lint and format checks CI runs, configured in [`.golangci.yml`](.golangci.yml) |

`make quality-check` also runs `gofumpt`, `goimports`, `staticcheck` and `gosec` if they are installed, and skips each one with a notice if it isn't.

## Setup

Fork the repository, clone your fork, and run this from the repository root:

```bash
make setup-all
```

It does three things:

1. **Installs the pre-commit hook.** It copies `.githooks/pre-commit` into your clone's Git hooks directory. Because it is a copy, run `make install-hooks` again whenever `.githooks/pre-commit` changes.
2. **Fetches the Go dependencies.** It runs `go mod tidy` and `go mod download` on the root module, once for each Go service. `go mod tidy` rewrites `go.mod` and `go.sum` if they are out of step with the code. The separate `tests/integration` module is not set up.
3. **Installs the Consent Portal's dependencies.** It runs `pnpm install --frozen-lockfile --filter consent-portal...` in the `portals` workspace, which fails rather than change `portals/pnpm-lock.yaml`. The paused Admin and Member portals are skipped.

It does not install the tools listed under [Prerequisites](#prerequisites), create any service's `config.yaml`, or start a database. To set up a single component, run `make setup <service>`, where `<service>` is `portal-backend`, `orchestration-engine`, `consent-engine`, `policy-decision-point` or `consent-portal`.

### The pre-commit hook

On each commit, the hook finds the maintained services that have staged files in their directories: `cmd/<svc>` and `internal/<svc>` for the Go services (plus `internal/database` for the Consent Engine), and `portals/apps/consent` for the Consent Portal. For each one it runs `make quality-check`, `make validate-build` and `make validate-test`, and it blocks the commit if any of them fails. A commit that touches none of those directories, such as a documentation change, an edit to `go.mod`, or a change to a shared package like `internal/authn`, skips the checks.

For the Go services, `make quality-check` formats code in place. The hook doesn't stage what it changes, so run `git status` after committing and commit any formatting changes it left behind.

## How Can I Contribute?

### Reporting Bugs

Found a bug? Please help us fix it! See our [Reporting Issues Guide](docs/contributing/reporting-issues.md) for details on how to report bugs effectively.

### Suggesting Enhancements

Have an idea for a new feature or improvement? We'd love to hear it! Check out our [Reporting Issues Guide](docs/contributing/reporting-issues.md) to learn how to submit feature requests.

### Pull Requests

Ready to submit code changes? Please review our [Pull Request Guidelines](docs/contributing/pull-requests.md) before submitting your PR.

### Development Setup

New to the project? Start with our [Development Guide](docs/contributing/development.md) to set up your local development environment.

## Quick Links

-   [**Reporting Issues**](docs/contributing/reporting-issues.md): How to report bugs and request features
-   [**Pull Requests**](docs/contributing/pull-requests.md): Guidelines for submitting code changes
-   [**Development Guide**](docs/contributing/development.md): Instructions for setting up your development environment and running tests
-   [**Code of Conduct**](CODE_OF_CONDUCT.md): Our community standards and expectations

## Getting Help

If you have questions or want to discuss ideas:

-   Check our [Issues](https://github.com/OpenNDX/openndx-core/issues) to see if your question has been asked before
-   Open a new [Issue](https://github.com/OpenNDX/openndx-core/issues/new) for bugs or feature requests
-   Review open [Pull Requests](https://github.com/OpenNDX/openndx-core/pulls) to see what others are working on

## Recognition

Contributors will be recognized in our project documentation and release notes. Thank you for helping make OpenNDX Core better!

We look forward to your contributions!
