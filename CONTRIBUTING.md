# Contributing to OpenNDX Core

Thank you for your interest in contributing to OpenNDX Core. We welcome contributions from everyone, whether you are fixing a bug, improving the documentation, or proposing a new feature.

This guide explains how to set up a development environment, how to run the tests and quality checks, and how to submit your changes for review.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold it. Please report unacceptable behavior to the project maintainers.

## Before You Start

- Search the [existing issues](https://github.com/openndx/openndx-core/issues?q=is%3Aissue) and [pull requests](https://github.com/openndx/openndx-core/pulls) to make sure that nobody is already working on the same change.
- For a significant change, open an issue first, so that the approach can be agreed on before you invest time in it.
- If you are new to the project, issues labeled [`good first issue`](https://github.com/openndx/openndx-core/labels/good%20first%20issue) are a good place to begin.
- The [README](README.md) introduces the components. Each component has its own README beside its code, such as [`cmd/pdp/README.md`](cmd/pdp/README.md), which explains how to run and configure it.

## Prerequisites

| Tool | Version | Used for |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.26.4 or later, as set in [`go.mod`](go.mod) | Every service and the `ondx` CLI |
| A C compiler, such as gcc or clang | Any recent version | The Go unit tests, which use SQLite through cgo, and the race detector that `make validate-test` enables |
| Git, Make and a POSIX shell | Any recent version | The `make` commands and the pre-commit hook. On Windows, use WSL. |
| [Node.js](https://nodejs.org/) and [pnpm](https://pnpm.io/installation) | Node 24 or later, and pnpm 11, as pinned in [`portals/package.json`](portals/package.json) | The Consent Portal, and `make setup-all`, which installs its dependencies |
| [Docker](https://docs.docker.com/get-docker/) with Compose v2 | Any recent version | The full local stack and the integration tests. The unit tests do not need it. |
| [golangci-lint](https://golangci-lint.run/welcome/install/) | v2.14.0, the version that CI uses | The lint and format checks that CI runs, configured in [`.golangci.yml`](.golangci.yml) |

If `go env CGO_ENABLED` prints `0`, Go did not find a C compiler, and the unit tests will not build.

`make quality-check` also uses `gofumpt`, `goimports`, `staticcheck` and `gosec` when they are installed. Without `gofumpt`, it formats code with `gofmt`; each of the other tools is skipped with a notice. `make install-tools` installs the latest versions of `gofumpt`, `goimports` and `staticcheck`. It does not install `gosec`, which you can install with:

```bash
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

`go install` places these tools in `$(go env GOPATH)/bin`, so make sure that directory is on your `PATH`.

## Setting Up

Fork the repository, clone your fork, and run the following command from the repository root:

```bash
make setup-all
```

This command does three things:

1. **It installs the pre-commit hook.** It copies `.githooks/pre-commit` into your clone's Git hooks directory. Because the hook is a copy, run `make install-hooks` again whenever `.githooks/pre-commit` changes.
2. **It fetches the Go dependencies.** It runs `go mod tidy` and `go mod download` on the root module, once for each Go service. Note that `go mod tidy` rewrites `go.mod` and `go.sum` if they are out of step with the code. The separate `tests/integration` module is not set up.
3. **It installs the Consent Portal's dependencies.** It runs `pnpm install --frozen-lockfile --filter consent-portal...` in the `portals` workspace, which fails rather than changes `portals/pnpm-lock.yaml`. The paused Admin and Member portals are skipped.

It does not install the tools listed under [Prerequisites](#prerequisites), create a `config.yaml` for any service, or start a database.

To set up a single component, run `make setup <service>`, where `<service>` is one of `portal-backend`, `orchestration-engine`, `consent-engine`, `policy-decision-point` or `consent-portal`.

### Running the Services

To run the full stack locally, run `docker compose up --build`, as described in the [README](README.md).

To run a single Go service natively, copy its `cmd/<svc>/config.example.yaml` to `cmd/<svc>/config.yaml`, which Git ignores, and run `make run <service>`. The service runs in the foreground until you press Ctrl+C. For the Consent Portal, `make run consent-portal` starts the development server. Each component's README lists its configuration keys and the services it depends on.

### The Pre-commit Hook

On each commit, the hook identifies the maintained services that have staged files in their directories: `cmd/<svc>` and `internal/<svc>` for the Go services (and `internal/database` for the Consent Engine), and `portals/apps/consent` for the Consent Portal. For each of these services, it runs `make quality-check`, `make validate-build` and `make validate-test`, and it blocks the commit if any of them fails.

A commit that touches none of those directories skips the checks. This includes documentation changes, edits to `go.mod`, and changes to shared packages such as `internal/authn`.

For the Go services, `make quality-check` formats the code in place, but the hook does not stage the files it changes. After each commit, run `git status` and commit any formatting changes that were left behind.

## Running Tests and Quality Checks

Run these commands for one component at a time, using the `<service>` names listed under [Setting Up](#setting-up). For the full list of commands, run `make help`.

```bash
make validate-test <service>    # unit tests
make quality-check <service>    # formatting and static checks, then the unit tests
make validate-build <service>   # build
```

For a Go service:

- `make validate-test` runs `go mod tidy`, then `go test -race` with coverage over the service's `cmd/<svc>` and `internal/<svc>` packages. It writes `coverage-<service>.out` and `coverage-<service>.html` to the repository root. Git ignores both files.
- `make quality-check` runs these steps in order:
  1. `make format`, which rewrites files with `gofumpt` and `goimports`;
  2. `go vet` and a `gofmt -l` check, either of which fails the command;
  3. `staticcheck` and `gosec`, whose findings are printed but do not fail the command;
  4. `make validate-test`.

  Each of the first three steps is also available on its own, as `make format`, `make lint`, `make staticcheck` and `make security`.
- `make validate-build` runs `go build` with `CGO_ENABLED=0` and writes the binary to `bin/<service>`.

For the Consent Portal, `make validate-test` runs `tsc --noEmit` and `eslint .`, `make quality-check` runs `eslint .`, and `make validate-build` runs `pnpm run build`, which writes to `portals/apps/consent/dist/`.

`make help` also lists batch commands, such as `validate-test-all` and `quality-check-all`. These can exit successfully even when a service fails, so read their output carefully, or use the per-service commands instead.

### Unit Tests

To run a single test:

```bash
go test ./internal/pdp/fieldpolicy -run TestService_PatchPolicy -count=1
```

Keep the following in mind:

- The unit tests do not need a running database. The Consent Engine, Policy Decision Point and Portal Backend tests use in-memory SQLite, with helpers in `internal/pdp/pdptest` and `internal/pb/database/dbtest`.
- The PostgreSQL tests in `cmd/ce` and `internal/database` are skipped unless `TEST_DATABASE_URL` points to a disposable PostgreSQL database with `cmd/ce/schema.sql` applied.
- Some Portal Backend tests expect nothing to be listening on `localhost:8082`, the Policy Decision Point's port.
- The Orchestration Engine's golden tests compare its output with the files in `internal/oe/federator/testdata/golden`. After an intentional change, regenerate the files with `go test ./internal/oe/federator -run TestFederateQueryGolden -update`, and review the diff before committing it.
- `TestLooksLikeIDImprovedLogic` in `internal/monitoring` currently fails on `main`.

### Linting the Way CI Does

CI runs golangci-lint v2.14.0 with [`.golangci.yml`](.golangci.yml), and reports only the issues on lines that the pull request changes. Because `make quality-check` does not run golangci-lint, run it yourself before you push:

```bash
golangci-lint run --new-from-rev=HEAD ./internal/... ./cmd/...
```

The `--new-from-rev=HEAD` option limits the report to your uncommitted changes.

### Integration Tests

The integration tests live in `tests/integration`, which is a separate Go module, so `go test ./...` from the repository root does not run them. To run them, start Docker and run:

```bash
cd tests/integration && ./run-local-tests.sh
```

The script starts the databases and services defined in `docker-compose.db.yml` and `docker-compose.test.yml`, runs `go test -race ./...`, and then removes the containers and their volumes. It stops early if `tests/integration/go.mod` or `go.sum` has uncommitted changes, unless `SKIP_GO_MOD_CHECK` is set.

Many of the tests are skipped, rather than failed, when a database or service they need is unreachable, so check the skip messages in the output.

## Making Changes

- Keep each pull request focused on a single issue. If a change is large, consider splitting it into a series of smaller pull requests.
- Add or update tests for any behavior that you add or change.
- Update the documentation that your change affects, including the component's README.
- Each service describes its API in `cmd/<svc>/openapi.yaml`. Any change to a route, a request or response body, a query parameter, a status code, an error code, a required scope or a validation rule must update the specification in the same pull request. The specification describes what the code does. If the code and the specification disagree because the code is wrong, file an issue rather than documenting the intended behavior as if it had shipped.
- The Admin and Member portals are paused prototypes. Please do not change them.

## Branches and Commits

Name your branch `<type>/<issue#>-<slug>`, using one of the commit types below and the number of the issue that the branch addresses. For example: `fix/501-validate-trace-id` or `docs/780-consolidate-contributing`.

Commit titles and pull request titles follow the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) specification:

```text
<type>(<scope>)!: <description>
```

- **Type** is the kind of change. This repository uses `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `ci` and `style`.
- **Scope** is the component that the change affects, such as `oe`, `pdp`, `ce`, `pb` or `ondx`, or another area, such as `deploy`, `authn` or `release`. Separate multiple scopes with commas, as in `refactor(pdp,pb)`. Changes that affect the whole repository, such as a formatting pass, omit the scope.
- **`!`** marks a breaking change, such as a removed or renamed endpoint. Omit it otherwise. For a breaking change, add a `BREAKING CHANGE:` footer to the commit body that explains what users of the change must do.
- **Description** is written in lowercase and in the imperative mood, without a final period. For example: `refactor(pdp,pb)!: rename the policy metadata API to field policies`.

The `main` branch accepts only squash merges, so each pull request lands on `main` as a single commit, to which GitHub appends the pull request number, as in `(#774)`. No CI check enforces this format, so please follow it carefully.

## Submitting a Pull Request

Work on a fork, and open your pull request against the `main` branch of `openndx/openndx-core`:

```bash
git remote add upstream https://github.com/openndx/openndx-core.git   # once, in your clone
git fetch upstream
git switch -c fix/501-validate-trace-id upstream/main
# make and commit your changes
git push -u origin fix/501-validate-trace-id
```

If `main` moves on and your branch needs the new changes, or conflicts with them, rebase your branch with `git fetch upstream && git rebase upstream/main`, and push it with `git push --force-with-lease`.

When you open the pull request:

- Give it a title in the [commit format](#branches-and-commits).
- Fill in the [pull request template](.github/pull_request_template.md), which GitHub adds to the description when you open the pull request in the browser. Keep every section. Where a section does not apply, say so, for example: "Not applicable; no UI changes."
- Under Related Issues, use `Closes #<n>` or `Fixes #<n>` to close the issue when the pull request is merged, or `Related to #<n>` to link it without closing it.

### Checks

Each component's validate workflow runs when a pull request changes that component's files, as defined by the `paths` filters in `.github/workflows/*-validate.yml`.

- For a Go service, the workflow checks that `go mod tidy` leaves `go.mod` and `go.sum` unchanged, runs golangci-lint on the changed lines, builds the service, runs the unit tests, and builds the Docker image and scans it with Trivy.
- For the Consent Portal, the workflow runs `eslint`, and builds and scans the Docker image.

The integration tests run when a pull request changes the Orchestration Engine, the Policy Decision Point, the Consent Engine, `internal/database`, `internal/monitoring`, `internal/utils`, `go.mod`, `go.sum` or `tests/integration`. A TruffleHog secret scan runs on every pull request, and CodeRabbit, an automated reviewer, also comments on pull requests.

### Review and Merge

The `main` branch requires:

- at least one approving review;
- approval of the latest push by someone other than the person who pushed it, so a push made after an approval needs a new approval;
- the resolution of every review conversation.

Once these requirements are met, the pull request is squash-merged into `main` as a single commit.

## Reporting Issues

Before you open an issue, search the [existing issues](https://github.com/openndx/openndx-core/issues?q=is%3Aissue) to see whether it has already been reported. To open a new issue, use one of the [issue forms](https://github.com/openndx/openndx-core/issues/new/choose): Report a Bug, Improvement Request or New Feature Request. Blank issues are disabled.

## Reporting Security Vulnerabilities

Please do not report security vulnerabilities through public issues or pull requests. Follow the process in [SECURITY.md](SECURITY.md) instead.

## Getting Help

If you have a question, or would like to discuss an idea before opening an issue, start a conversation in [GitHub Discussions](https://github.com/openndx/openndx-core/discussions).

## Recognition

The release notes for each version credit the author of every pull request that it includes. Thank you for helping to make OpenNDX Core better.
