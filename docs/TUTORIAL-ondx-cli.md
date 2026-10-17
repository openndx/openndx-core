# Tutorial: installing and using the `ondx` CLI

`ondx` is a Go CLI for OpenNDX management operations — members, schemas, applications, and
policies — against the Portal Backend API. This tutorial covers getting the binary onto your
machine and walking through a full onboarding scenario. For the exhaustive flag-by-flag
reference, see [`cmd/ondx/README.md`](../cmd/ondx/README.md); this doc is the narrative,
start-to-finish version.

For a local demo that continues through mock-provider queries and citizen
consent approval, use the [end-to-end walkthrough](../dev/WALKTHROUGH.md).

## Prerequisites

- Network access to an OpenNDX Portal Backend instance and ThunderID, the identity provider it
  trusts
- Go 1.26+ only if you install with `go install` or build from source (Options D and E below)

## 1. Install

Prebuilt `ondx` binaries for Linux, macOS, and Windows (amd64/arm64) are attached to every
[GitHub Release](https://github.com/openndx/openndx-core/releases) from the first release that
includes the CLI onward.

### Option A — Homebrew (macOS / Linux)

```bash
brew install openndx/tap/ondx
```

`brew upgrade ondx` picks up new releases.

### Option B — install script (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/openndx/openndx-core/main/scripts/install.sh | sh
```

The script picks the build for your OS/CPU, verifies it against the release's `checksums.txt`,
and installs it to `/usr/local/bin` if that's writable, otherwise `~/.local/bin`. Set
`ONDX_VERSION=v0.3.0` to pin a release, or `ONDX_INSTALL_DIR=<dir>` to install somewhere else.

To download manually instead, grab `ondx_<os>_<arch>.tar.gz` (`.zip` on
Windows) from the release page. Every release archive carries a build provenance attestation you
can verify with the GitHub CLI:

```bash
gh attestation verify ondx_linux_amd64.tar.gz --repo openndx/openndx-core
```

### Option C — Windows (PowerShell script)

Run the install script from PowerShell:

```powershell
irm https://raw.githubusercontent.com/openndx/openndx-core/main/scripts/install.ps1 | iex
```

It picks the amd64 or arm64 build, verifies it against the release's `checksums.txt`, installs
`ondx.exe` to `%LOCALAPPDATA%\Programs\ondx`, and adds that folder to your user `PATH` (open a new
terminal afterwards). Set `$env:ONDX_VERSION = 'v0.3.0'` to pin a release, or
`$env:ONDX_INSTALL_DIR` to install somewhere else. Re-run it to upgrade.

The binary isn't code-signed, so if you download the zip through a browser instead, Windows
SmartScreen may warn on first run.

### Option D — `go install`

This fetches, builds, and installs the CLI without cloning the repo:

```bash
go install github.com/openndx/openndx-core/cmd/ondx@latest
```

It's installed to `GOBIN` if you've set that, otherwise `$(go env GOPATH)/bin` (usually
`~/go/bin`) — make sure that directory is on your `PATH`.

> **Note:** `@latest` resolves to the newest release tag, so it only works once a tag that
> includes `cmd/ondx` has been cut. Until then, use `@main` (or pin a commit, `@<sha>`).

### Option E — clone and build from source

Useful if you're working inside this repo already, or want a specific commit/branch without
relying on the module proxy:

```bash
git clone https://github.com/openndx/openndx-core.git
cd openndx-core
go build -o ondx ./cmd/ondx
```

This writes `ondx` to the current directory (not your `PATH`), so invoke it as `./ondx ...` in
every command below, or move it onto your `PATH` yourself.

Or skip the build step entirely and run it straight from source with `go run ./cmd/ondx ...` in
place of `./ondx ...`.

### Check the install

```bash
ondx version
```

## 2. Log in

```bash
ondx login
```

`ondx` ships with a built-in `local` profile matching this repo's `docker compose` local-dev
stack (ThunderID as IDP on `http://localhost:8090`, client `NDX_CLI`, callback port `8765`,
Portal Backend at `http://localhost:8083`), so a bare `ondx login` works out of the box against
it once that stack is running.

This opens your browser to the identity provider's login page (Authorization Code + PKCE, a
similar browser-based login to what `gh auth login` uses), catches the redirect on a local
callback server, exchanges the code for a token, and caches it at `~/.openndx/credentials.json`.
Every other command reuses that cached token automatically, refreshing it as needed — you only
need to log in again once the refresh token itself expires.

Against a different environment, either pass flags explicitly — including `--insecure=false` if
your current profile is `local` (which defaults it to `true`) and you're pointing at a real TLS
endpoint:

```bash
ondx login --issuer https://idp.example.com --client-id ondx-cli --scopes "openid roles email" \
  --insecure=false
```

or set up a named profile once and reuse it (see [Profiles](#4-profiles-for-multiple-environments)
below).

## 3. Walk through a real scenario

This walkthrough uses two members: **DRP** (Department of Registrar of Persons), which owns and
provides a schema, and **DIE** (Department of Immigration and Emigration), which registers an
application and requests access to DRP's schema fields.

### 3.1 Onboard the schema-owning member (DRP)

```bash
ondx members create --name "Department of Registrar of Persons" --email drp@drp.gov.lk \
  --phone "+1234567890" --ou-id 01900000-0000-7000-8000-0000000000c0 \
  --pb-url http://localhost:8083
```

This prints a `memberId` — keep it, you'll need it as `<drpMemberId>` below.

A member is an organization: `--ou-id` points at the ThunderID organization unit DRP's users
belong to. Portal Backend doesn't create organization units or users. The local stack seeds a
`drp` and a `die` organization unit (`thunderid/bootstrap/users.yaml`), each with a staff user
(`drp-admin` / `die-admin`, password `1234`). For any other organization, create the organization
unit and its users in ThunderID's console first and pass its ID here.

### 3.2 Register DRP's schema

```bash
ondx schemas create --schema-id drp-schema-v1 \
  --name "DRP Person Registry" --endpoint http://drp.example.gov.lk/graphql \
  --member-id <drpMemberId> \
  --field person.fullName:public:primary \
  --field person.otherNames:public:primary \
  --field person.permanentAddress:restricted:primary \
  --field person.profession:restricted:primary \
  --pb-url http://localhost:8083
```

This prints each field as `schemaId:fieldName` — copy that shape directly into the next steps.
A field must be registered here before any application can be granted it.

`--schema-id drp-schema-v1` matches the ID the Orchestration Engine's unified schema already uses
for DRP (`@sourceInfo(schemaId: "drp-schema-v1", ...)` in `cmd/oe/schema.graphql`), so the OE's
policy checks find this schema's fields without editing its config. Leave `--schema-id` out and
Portal Backend generates a UUID instead. The ID can't be changed later.

### 3.3 Onboard the application-owning member (DIE)

```bash
ondx members create --name "Department of Immigration and Emigration" --email die@die.gov.lk \
  --phone "+1234567890" --ou-id 01900000-0000-7000-8000-0000000000c1 \
  --pb-url http://localhost:8083
```

Keep the printed `memberId` as `<dieMemberId>`.

### 3.4 Register DIE's application and grant it DRP's fields

A passport application needs the applicant's full name and permanent address:

```bash
ondx applications create --name "Passport Application" --member-id <dieMemberId> \
  --field <schemaId>:person.fullName --field <schemaId>:person.permanentAddress \
  --pb-url http://localhost:8083
```

This prints an `applicationId`, plus the application's IdP client ID and secret. Portal Backend
creates that M2M client in DIE's ThunderID organization unit and grants it the `getdata` role, so
DIE can call the Orchestration Engine with a `client_credentials` token. **Store the secret now** —
it's shown only once. `applications create` grants only the fields passed at creation time.

To register a client you already onboarded in ThunderID yourself instead, pass
`--idp-application-id <thunder-app-id> --idp-client-id <thunder-client-id>`.

### 3.5 Change what's granted later

`policy update` replaces Portal Backend's saved field list and adds or refreshes
the supplied PDP grants. Pass every field the saved list should contain.
Omitting a field does not currently revoke its existing PDP grant. Inspect the
saved list first:

```bash
ondx applications get --app-id <applicationId> --pb-url http://localhost:8083
```

Then, to add `person.profession` while keeping the two fields already granted:

```bash
ondx policy update --app-id <applicationId> \
  --field <schemaId>:person.fullName \
  --field <schemaId>:person.permanentAddress \
  --field <schemaId>:person.profession \
  --pb-url http://localhost:8083
```

## 4. Profiles, for multiple environments

Rather than repeating `--issuer`, `--client-id`, `--pb-url`, etc. on every command, save them
once as a named profile:

```bash
ondx profile set staging --issuer https://idp.staging.example.com \
  --client-id ondx-cli-staging --pb-url https://pb.staging.example.com
ondx profile use staging      # makes staging the default from here on
ondx profile list             # * marks the current profile
ondx login --profile local    # one-off override back to local, without switching the default
```

Profiles live in `~/.openndx/config.json`, and each non-`local` profile gets its own credentials
cache (`~/.openndx/credentials-<name>.json`) so switching profiles never reuses a token cached
against a different identity provider.

## 5. Command reference (quick view)

| Command                                                   | Purpose                                             |
|-----------------------------------------------------------|-----------------------------------------------------|
| `ondx login [flags]`                                      | Browser-based OAuth2 login; caches a token          |
| `ondx profile list` / `use <name>` / `set <name> [flags]` | Manage named environments                           |
| `ondx members create [flags]`                             | Register a member                                   |
| `ondx members list [--ou-id ...] [--json]`                | List members                                        |
| `ondx members get --member-id ... [--json]`               | Show a member's details                             |
| `ondx schemas create [flags]`                             | Register a schema and its grantable fields          |
| `ondx schemas list [--member-id ...] [--json]`            | List schemas                                        |
| `ondx schemas get --schema-id ... [--json]`               | Show a schema's details and fields                  |
| `ondx applications create [flags]`                        | Register an application, optionally granting fields |
| `ondx applications list [--member-id ...] [--json]`       | List applications                                   |
| `ondx applications get --app-id ... [--json]`             | Show an application's details and current policy    |
| `ondx policy update --app-id ... --field ... [flags]`     | Save the selected fields and add or refresh grants  |

Every command that calls Portal Backend (`members create`/`list`/`get`, `schemas create`/`list`/`get`,
`applications create`/`list`/`get`, `policy update`) also accepts `--pb-url`, `--credentials-path`,
`--insecure`, and `--profile`. `ondx login` accepts `--credentials-path`, `--insecure`, and
`--profile`, but not `--pb-url` (it doesn't talk to Portal Backend). `ondx profile list` and
`ondx profile use <name>` take no flags at all. Run `ondx <command> -h` for the full flags on any
of them, or see the [flag tables in `cmd/ondx/README.md`](../cmd/ondx/README.md#commands) for
complete detail (including env var equivalents for every flag).

## 6. Troubleshooting

- **`policy metadata not found`** on `applications create`/`policy update` — the `--field` you
  passed was never registered via `schemas create`. Register it there first.
- **Connection errors against `https://localhost:8090`** — ThunderID's local-dev instance serves
  plain HTTP. Use `http://localhost:8090`, and update any older `.env` or `~/.openndx/config.json`
  overrides that still point at `https://`.
- **`ondx login` rejects your `--callback-port`** — ThunderID's `NDX_CLI` client has a redirect
  URI pinned to `http://127.0.0.1:8765/callback`; `ondx` enforces port `8765` for that client
  instead of attempting a login the IDP would reject anyway.
- **Application creation fails with a ThunderID error** — Portal Backend creates the client with
  its own `PORTAL_BACKEND` client (`thunderid/bootstrap/application.yaml`). On a ThunderID
  database bootstrapped before that client existed, reset the stack (`docker compose down -v`).
  `Application already exists` means ThunderID already has an application with that name.
- **Member creation fails without `--ou-id`** — create the member's organization unit in
  ThunderID's console first and pass its ID.

For anything not covered here — full flag tables, the TLS/callback-port/ThunderID-resource-binding
notes, and current limitations — see [`cmd/ondx/README.md`](../cmd/ondx/README.md).
