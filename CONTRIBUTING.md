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
| A C compiler, such as gcc or clang | Any recent version | The Go unit tests, which use SQLite through cgo, and the race detector that `make test` enables |
| Git, GNU Make and curl | Any recent version | The `make` commands and the pre-commit hook. `make` downloads golangci-lint with curl. |
| [Node.js](https://nodejs.org/) and [pnpm](https://pnpm.io/installation) | Node 24 or later, and pnpm 11, as pinned in [`portals/package.json`](portals/package.json) | The Consent Portal, and `make setup`, which installs its dependencies |
| [Docker](https://docs.docker.com/get-docker/) with Compose v2 | Any recent version | The local stack (`make up`, `make deps`) and the integration tests. The unit tests do not need it. |

On macOS, the Xcode Command Line Tools (`xcode-select --install`) provide Make, Git and clang.

On Windows, install GNU Make (for example `choco install make`) and [Git for Windows](https://git-scm.com/download/win). The Makefile runs its recipes with the `sh` that ships with Git, so `make` works from PowerShell, cmd or Git Bash.

If `go env CGO_ENABLED` prints `0`, Go did not find a C compiler, and the unit tests will not build.

golangci-lint needs no installing. `GOLANGCI_LINT_VERSION` in the [`Makefile`](Makefile) pins its version. The first `make setup`, `make fmt` or `make lint` downloads that release's binary, the same one CI runs, into `bin/`, and checks it against the release's SHA-256 checksums. After a version bump, the next run downloads the new release. The CI validate workflows read the version from the Makefile, so a version bump changes only that line.

## Setting Up

Fork the repository, clone your fork, and run the following command from the repository root:

```bash
make setup
```

This command does four things:

1. **It installs the pre-commit hook.** It points Git at the `.githooks` directory (`git config core.hooksPath .githooks`), so a change to the hook takes effect without installing it again.
2. **It fetches the Go dependencies.** It runs `go mod download`, which does not change `go.mod` or `go.sum`. The separate `tests/integration` module is not set up.
3. **It downloads golangci-lint** at the pinned version.
4. **It installs the Consent Portal's dependencies.** It runs `pnpm install --frozen-lockfile --filter consent-portal...` in the `portals` workspace, which fails rather than changes `portals/pnpm-lock.yaml`. The paused Admin and Member portals are skipped.

Run it again after pulling changes to the portal's dependencies. It does not create a `config.yaml` for any service or start a database.

### The Pre-commit Hook

On every commit, the hook first adds the [license header](#license-headers) to staged files that lack it, and stages the result. Then, when the commit stages a `.go` file, `go.mod`, `go.sum` or `.golangci.yml`, it runs `make check` and blocks the commit if it fails. `make check` changes no files. It checks the working tree, so unstaged changes count too. Other commits, such as documentation changes, skip `make check`.

To skip the hook for one commit, run `git commit --no-verify`.

## Running the Platform

A single service rarely runs on its own: the Orchestration Engine needs the Policy Decision Point, the Consent Engine, ThunderID and data providers, and the Portal Backend needs PostgreSQL, ThunderID and the Policy Decision Point. There are two supported ways to run the platform.

### Full Stack in Docker

```bash
make up      # build and start everything, plus the mock data providers
make down    # stop it
```

This is the default for trying the platform; the [end-to-end walkthrough](dev/WALKTHROUGH.md) uses it. A bare `docker compose up --build`, as in the [README](README.md), starts the same stack without the [mock providers](dev/README.md).

### Dependencies in Docker, Services Native

This is the development loop: fast rebuilds, a debugger and plain logs.

```bash
make deps    # PostgreSQL (with every schema.sql loaded), ThunderID, the audit service, the mock providers
```

PostgreSQL loads the schemas only when its volume is first created. After pulling a `schema.sql` change, load them again into the running database:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d postgres -f /docker-entrypoint-initdb.d/init-db.sql'
```

Every `schema.sql` is safe to re-run: this adds missing tables and indexes, and skips those that exist. A change to an existing table therefore needs a fresh database: `docker compose down -v` deletes the stack's volumes, ThunderID's data included, and the next `make deps` starts from scratch.

Then run each service in a terminal of its own, from its directory, where it reads `config.yaml`:

```bash
cd cmd/pdp && go run .
cd cmd/ce && go run .
cd cmd/oe && go run .
cd cmd/pb && go run .
cd portals/apps/consent && pnpm dev    # reads public/config.js
```

Before the first run, create the configuration files, which Git ignores, from the examples next to them:

| Copy | To | Then |
|---|---|---|
| `cmd/pdp/config.example.yaml` | `cmd/pdp/config.yaml` | |
| `cmd/ce/config.example.yaml` | `cmd/ce/config.yaml` | |
| `cmd/oe/config.example.yaml` | `cmd/oe/config.yaml` | |
| `cmd/pb/config.example.yaml` | `cmd/pb/config.yaml` | |
| `portals/apps/consent/public/config.example.js` | `portals/apps/consent/public/config.js` | Set `idpClientId: 'CONSENT_PORTAL'` and `idpBaseUrl: 'http://localhost:8090'` |

The example configurations point at the ports that `make deps` publishes on `localhost`, and read their secrets from the environment. An `export` lasts only for its terminal, so set them in each terminal you start a service from. With the Compose defaults:

```bash
export DB_PASSWORD=1234          # PowerShell: $env:DB_PASSWORD = "1234"
export IDP_CLIENT_SECRET=1234    # the Portal Backend's ThunderID client secret
```

If something else on your machine already uses port 5432, such as a local PostgreSQL installation, set `DB_PORT` in `.env` to a free port before `make deps`, and use that port in the configurations. Each component's README lists its configuration keys.

### One Service Native, the Rest in Docker

Native services reach containers through ports on `localhost`, but containers reach each other by service name, never through the host. So this works only for a service that no container calls: the Orchestration Engine and the Portal Backend.

```bash
make up
docker compose stop orchestration-engine
cd cmd/oe && go run .
```

`make up` starts every service in Docker, on the same host ports that the native services use. Stop any service you run natively before you run `make up`, or Docker cannot publish that port and `make up` stops before it starts the mock providers.

Running the Policy Decision Point or the Consent Engine natively means running their callers natively too, which is the previous mode.

## Running Tests and Quality Checks

Every Go target works on the whole module. For the full list of commands, run `make help`.

| Command | What it does |
|---|---|
| `make build` | `go build ./...` |
| `make test` | `go test -race ./...`. To test some packages, run for example `make test PKG=./internal/pdp/...`. |
| `make test-integration` | Runs the [integration tests](#integration-tests) against services in Docker |
| `make fmt` | Formats Go code with `gofumpt` and `goimports` |
| `make lint` | Runs golangci-lint with [`.golangci.yml`](.golangci.yml): vet, staticcheck, gosec and the rest. `LINT_FLAGS` passes extra flags. |
| `make license` | Adds the [license header](#license-headers) to files that lack it |
| `make license-check` | Lists the files without the license header, and fails if there are any, as CI does |
| `make check` | Checks that `go.mod` and `go.sum` are tidy, then runs `make license-check`, `make lint` and `make test`. It changes no files. |
| `make clean` | Removes `bin/` and the coverage files |

Before you push, run `make fmt` and `make check`.

### Unit Tests

To run a single test:

```bash
go test ./internal/pdp/fieldpolicy -run TestService_PatchPolicy -count=1
```

Keep the following in mind:

- The unit tests do not need a running database. The Consent Engine, Policy Decision Point and Portal Backend tests use in-memory SQLite, with helpers in `internal/pdp/pdptest` and `internal/pb/database/dbtest`.
- The PostgreSQL tests in `cmd/ce` and `internal/database` are skipped unless `TEST_DATABASE_URL` points to a disposable PostgreSQL database with `cmd/ce/schema.sql` applied.
- The Orchestration Engine's golden tests compare its output with the files in `internal/oe/federator/testdata/golden`. After an intentional change, regenerate the files with `go test ./internal/oe/federator -run TestFederateQueryGolden -update`, and review the diff before committing it.

### Linting the Way CI Does

CI runs golangci-lint at the version in the [`Makefile`](Makefile) with [`.golangci.yml`](.golangci.yml), and reports only the issues on lines that the pull request changes. `make lint` and `make check` report every issue in the module. To see only what CI will report:

```bash
git fetch upstream
make lint LINT_FLAGS=--new-from-merge-base=upstream/main
```

The `--new-from-merge-base=upstream/main` option limits the report to the lines your branch has changed since it left `main`, whether committed or not. While you work, `LINT_FLAGS=--new-from-rev=HEAD` checks only your uncommitted changes.

### Integration Tests

The integration tests live in `tests/integration`, which is a separate Go module, so `make test` does not run them. To run them, start Docker and run:

```bash
make test-integration
```

This runs `tests/integration/run-local-tests.sh`, which starts the databases and services defined in `docker-compose.db.yml` and `docker-compose.test.yml`, runs `go test -race ./...`, and then removes the containers and their volumes. It stops early if `tests/integration/go.mod` or `go.sum` has uncommitted changes, unless `SKIP_GO_MOD_CHECK` is set.

Many of the tests are skipped, rather than failed, when a database or service they need is unreachable, so check the skip messages in the output.

## Making Changes

- Keep each pull request focused on a single issue. If a change is large, consider splitting it into a series of smaller pull requests.
- Add or update tests for any behavior that you add or change.
- Update the documentation that your change affects, including the component's README.
- Each service describes its API in `cmd/<svc>/openapi.yaml`. Any change to a route, a request or response body, a query parameter, a status code, an error code, a required scope or a validation rule must update the specification in the same pull request. The specification describes what the code does. If the code and the specification disagree because the code is wrong, file an issue rather than documenting the intended behavior as if it had shipped.
- The Admin and Member portals are paused prototypes. Please do not change them.

### License Headers

Every source file starts with an SPDX license header, taken from `.github/license-header.tmpl`:

```text
SPDX-License-Identifier: Apache-2.0
Copyright (c) <year> Lanka Software Foundation
```

The header is written in the comment syntax of the file's type, for example `//` in Go, `#` in YAML and shell scripts, and `--` in SQL. `scripts/license-headers.sh` decides which files need a header: every file that Git tracks and whose type has a comment syntax, except the Admin and Member portals, `pnpm-lock.yaml`, `testdata` directories, `node_modules` and `dist`. JSON and Markdown files have no header.

- `make license` adds the header to every file that lacks one, and `make license-check` lists those files and fails, as CI does.
- The pre-commit hook adds the header to staged files that lack it, and stages the result. If you staged only part of such a file, the hook stages the rest of it too.
- The License Headers workflow runs the same check on every pull request, whatever the pull request changes.
- In Helm templates, write the header as a template comment, `{{- /* ... */ -}}`, as the existing templates do, so that it does not appear in the rendered manifests.

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

The integration tests run when a pull request changes the Orchestration Engine, the Policy Decision Point, the Consent Engine, `internal/database`, `internal/monitoring`, `internal/utils`, `go.mod`, `go.sum` or `tests/integration`. A TruffleHog secret scan and the [license header check](#license-headers) run on every pull request, and CodeRabbit, an automated reviewer, also comments on pull requests.

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
