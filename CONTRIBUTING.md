# Contributing to OpenNDX Core

Thank you for your interest in contributing to the OpenNDX Core project! We welcome contributions from everyone and are grateful for your help in making this project better.

## Code of Conduct

This project adheres to a [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Prerequisites

| Tool | Version | Used for |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.26.4 or later, as set in [`go.mod`](go.mod) | every service and the `ondx` CLI |
| A C compiler (gcc or clang) | any recent | the Go unit tests, which use SQLite through cgo, and the race detector that `make validate-test` turns on; if `go env CGO_ENABLED` prints `0`, Go found no compiler |
| Git, Make and a POSIX shell | any recent | the `make` commands and the pre-commit hook; on Windows, use WSL |
| [Node.js](https://nodejs.org/) and [pnpm](https://pnpm.io/installation) | Node 24 or later; pnpm 11, pinned in [`portals/package.json`](portals/package.json) | the Consent Portal, and `make setup-all`, which installs its dependencies |
| [Docker](https://docs.docker.com/get-docker/) with Compose v2 | any recent | the full local stack and the integration tests; unit tests don't need it |
| [golangci-lint](https://golangci-lint.run/welcome/install/) | v2.14.0, the version CI pins | the lint and format checks CI runs, configured in [`.golangci.yml`](.golangci.yml) |

`make quality-check` also uses `gofumpt`, `goimports`, `staticcheck` and `gosec` if they are installed. Without `gofumpt` it formats with `gofmt`; each of the others is skipped with a notice. `make install-tools` installs the latest `gofumpt`, `goimports` and `staticcheck`, but not `gosec` or golangci-lint.

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

## Tests and quality checks

Run these for one component at a time, using the `<service>` names listed under [Setup](#setup). `make help` lists every command.

```bash
make validate-test <service>    # unit tests
make quality-check <service>    # formatting and static checks, then the unit tests
make validate-build <service>   # build
```

For a Go service:

- `make validate-test` runs `go mod tidy`, then `go test -race` with coverage over the service's `cmd/<svc>` and `internal/<svc>` packages. It writes `coverage-<service>.out` and `coverage-<service>.html` to the repository root; both are git-ignored.
- `make quality-check` runs, in order: `make format`, which rewrites files; `go vet` and a `gofmt -l` check, which fail the command; `staticcheck` and `gosec`, whose findings are printed but don't fail it; then `make validate-test`.
- `make validate-build` runs `go build` with `CGO_ENABLED=0` and writes the binary to `bin/<service>`.

For the Consent Portal, `make validate-test` runs `tsc --noEmit` and `eslint .`, `make quality-check` runs `eslint .`, and `make validate-build` runs `pnpm run build` into `portals/apps/consent/dist/`.

The batch commands in `make help`, such as `validate-test-all` and `quality-check-all`, can exit successfully even when a service fails, so read their output or use the per-service commands.

### Unit tests

To run a single test:

```bash
go test ./internal/pdp/fieldpolicy -run TestService_PatchPolicy -count=1
```

- Unit tests need no running database: the CE, PDP and PB tests use in-memory SQLite (helpers in `internal/pdp/pdptest` and `internal/pb/database/dbtest`). The PostgreSQL tests in `cmd/ce` and `internal/database` are skipped unless `TEST_DATABASE_URL` points to a disposable PostgreSQL database with `cmd/ce/schema.sql` applied.
- Some PB unit tests expect nothing to be listening on `localhost:8082`, the PDP's port.
- The OE federator golden tests compare output with the files in `internal/oe/federator/testdata/golden`. After an intentional change, regenerate them with `go test ./internal/oe/federator -run TestFederateQueryGolden -update` and review the diff.
- `TestLooksLikeIDImprovedLogic` in `internal/monitoring` fails on `main`.

### Linting as CI does

CI runs golangci-lint v2.14.0 with [`.golangci.yml`](.golangci.yml) and reports only issues on lines the pull request changes. `make quality-check` doesn't run golangci-lint, so run it yourself:

```bash
golangci-lint run --new-from-rev=HEAD ./internal/... ./cmd/...
```

`--new-from-rev=HEAD` limits the report to your uncommitted changes.

### Integration tests

`tests/integration` is a separate Go module, so `go test ./...` from the repository root doesn't run it. With Docker running:

```bash
cd tests/integration && ./run-local-tests.sh
```

The script starts the databases and services from `docker-compose.db.yml` and `docker-compose.test.yml`, runs `go test -race ./...`, then removes the containers and their volumes. It stops early if `tests/integration/go.mod` or `go.sum` has uncommitted changes, unless `SKIP_GO_MOD_CHECK` is set. Many tests skip rather than fail when a database or service they need is unreachable, so check the skip messages in the output.

## Branches and commits

Name branches `<type>/<issue#>-<slug>`, using a commit type below and the number of the issue the branch works on, for example `fix/501-validate-trace-id` or `docs/780-consolidate-contributing`.

Commit titles and pull request titles follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):

```text
<type>(<scope>)!: <description>
```

- **type**: the kind of change. The types used here are `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `ci` and `style`.
- **scope**: the component the change is in, such as `oe`, `pdp`, `ce`, `pb` or `ondx`, or another area such as `deploy`, `authn` or `release`. Separate several with commas, as in `refactor(pdp,pb)`. Repository-wide changes, such as a formatting pass, leave the scope out.
- **`!`**: marks a breaking change, such as a removed or renamed endpoint; leave it out otherwise. Describe what users of the change must do in a `BREAKING CHANGE:` footer in the commit body.
- **description**: lowercase, imperative and without a final period, for example `refactor(pdp,pb)!: rename the policy metadata API to field policies`.

`main` accepts only squash merges, so each pull request lands on `main` as one commit, with GitHub adding the pull request number, as in `(#774)`. No CI check enforces this format.

## Pull requests

Work on a fork and open pull requests against `main` in `openndx/openndx-core`:

```bash
git remote add upstream https://github.com/openndx/openndx-core.git   # once, in your clone
git fetch upstream
git switch -c fix/501-validate-trace-id upstream/main
# commit your changes
git push -u origin fix/501-validate-trace-id
```

If `main` moves on and your branch needs its changes or conflicts with it, rebase with `git fetch upstream && git rebase upstream/main` and push with `git push --force-with-lease`.

When you open the pull request:

- Give it a title in the [commit format](#branches-and-commits).
- Fill in [the template](.github/pull_request_template.md), which GitHub adds to the description when you open the pull request in the browser. Keep every section, and where one doesn't apply say so, for example "Not applicable; no UI changes."
- Under Related Issues, `Closes #<n>` or `Fixes #<n>` closes the issue when the pull request is merged; `Related to #<n>` only links it.

### Checks

Each component's validate workflow runs when the pull request changes that component's files, as set by the `paths` in `.github/workflows/*-validate.yml`. For a Go service it checks that `go mod tidy` leaves `go.mod` and `go.sum` unchanged, runs golangci-lint on the changed lines, builds the service, runs the unit tests, and builds and scans the Docker image with Trivy. For the Consent Portal it runs `eslint` and builds and scans the image. The integration tests run when the pull request changes the OE, PDP or CE, `internal/database`, `internal/monitoring`, `internal/utils`, `go.mod`, `go.sum` or `tests/integration`, and a TruffleHog secret scan runs on every pull request. CodeRabbit, an automated reviewer, also comments on pull requests.

### Review and merge

The rules on `main` require:

- at least one approving review;
- approval of the latest push by someone other than the person who pushed it, so a push after an approval needs a new one;
- every review conversation resolved.

`main` accepts only squash merges, so your branch lands as one commit.

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
