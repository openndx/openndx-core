# ondx CLI

A Go CLI for OpenNDX management operations — members, schemas, applications, and policies — against the Portal Backend API.

To use these commands against a fresh local stack and retrieve data from the
mock providers, follow the [end-to-end walkthrough](../../dev/WALKTHROUGH.md).
It includes policy setup, citizen consent, expected responses, and cleanup.

## Overview

`ondx` logs an operator in via a browser-based OAuth2 login (Authorization Code + PKCE, like `gh auth login`), caches the resulting token, and uses it to call Portal Backend's management endpoints: creating members, schemas, and applications, listing/inspecting members, schemas, and applications, and updating an application's policy (its granted schema fields).

It works against any OIDC-compatible identity provider PB is configured to trust. Today that's usually one of two setups:

- **ThunderID** (local dev) — the IDP this repo's `docker compose` stack runs locally. A member is an organization linked to a ThunderID organization unit: create the organization unit (and its users) in ThunderID first, then register the member with `--ou-id`. Creating an application provisions its M2M OAuth2 client in that member's organization unit, grants it the `getdata` role, and prints its client secret once. To register a client you already onboarded in ThunderID instead, pass `--idp-application-id --idp-client-id`.
- **Asgardeo (WSO2)** — the identity provider PB was originally built against. `ondx` can still log in against it, but Portal Backend now provisions application clients in ThunderID only.

## Quick Start

### 1. Install

```bash
brew install openndx/tap/ondx
# or
curl -fsSL https://raw.githubusercontent.com/openndx/openndx-core/main/scripts/install.sh | sh
# or, on Windows (PowerShell)
irm https://raw.githubusercontent.com/openndx/openndx-core/main/scripts/install.ps1 | iex
# or
go install github.com/openndx/openndx-core/cmd/ondx@latest
```

Prebuilt binaries for every OS/CPU are also attached to each [GitHub Release](https://github.com/openndx/openndx-core/releases) — see the [tutorial](../../docs/TUTORIAL-ondx-cli.md#1-install) for all options, including verifying downloads with `gh attestation verify`. From a checkout, `go build -o ondx ./cmd/ondx` (or `go run ./cmd/ondx ...`) works too. Run `ondx version` to see which build you have.

### 2. Log in

```bash
./ondx login
```

`ondx` ships with a built-in `local` profile matching this repo's `docker compose` local-dev stack (ThunderID as IDP on `http://localhost:8090`, client `NDX_CLI`, port `8765`, resource `http://pb.openndx.local`, Portal Backend at `http://localhost:8083`), so a bare `ondx login` works out of the box against it — see [Profiles](#ondx-profile-list--ondx-profile-use-name--ondx-profile-set-name-flags) to add other environments (staging, a partner's deployment, ...) or override any of these values.

This opens your browser to the identity provider's login page, catches the redirect on a local callback server, exchanges the code for a token, and caches it at `~/.openndx/credentials.json`. Every other command reuses that cached token automatically, refreshing it via its `refresh_token` when it's expired — you only need to log in again if the refresh token itself is no longer valid.

Every flag below can still be passed explicitly, which overrides whatever the active profile sets, e.g. to point at a different issuer without touching your profile:

```bash
./ondx login --issuer http://localhost:8090 --client-id NDX_CLI --callback-port 8765 --scopes "openid roles email"
```

Portal Backend checks an OAuth2 scope on every route (see [Authorization](../pb/README.md#authorization)), so an explicit `--scopes` must also list the PB scopes the commands you run need, e.g. `"openid roles email member:read member:write member:read-all member:write-all"` to manage every organization unit's members. The `local` profile requests every PB scope; ThunderID grants only the ones your roles hold.

`--issuer` fetches `authorization_endpoint`/`token_endpoint` from the identity provider's `{issuer}/.well-known/openid-configuration` (OIDC Discovery), so most standards-compliant IDPs only need a base URL. If an IDP doesn't serve that document at the standard root path, fall back to `--auth-url`/`--token-url` (which take precedence over discovery when set):

```bash
./ondx login --auth-url http://localhost:8090/oauth2/authorize --token-url http://localhost:8090/oauth2/token --client-id NDX_CLI --callback-port 8765 --scopes "openid roles email"
```

This walkthrough uses two members: **DRP** (Department of Registrar of Persons), which owns and provides a schema, and **DIE** (Department of Immigration and Emigration), which registers an application and requests access to DRP's schema fields.

### 3. Onboard the schema-owning member (DRP)

```bash
./ondx members create --name "Department of Registrar of Persons" --email drp@drp.gov.lk --phone "+1234567890" \
  --ou-id 01900000-0000-7000-8000-0000000000c0 --pb-url http://localhost:8083
# → prints memberId (drpMemberId)
```

### 4. Register DRP's schema

Fields taken from `cmd/oe/schema.graphql`'s `PersonInfo` type — the ones sourced from `drp` (`providerKey: "drp"`, `schemaId: "drp-schema-v1"`):

```bash
./ondx schemas create --schema-id drp-schema-v1 \
  --name "DRP Person Registry" --endpoint http://drp.example.gov.lk/graphql \
  --member-id <drpMemberId> \
  --field person.fullName:public:primary --field person.otherNames:public:primary \
  --field person.permanentAddress:restricted:primary --field person.profession:restricted:primary \
  --pb-url http://localhost:8083
# → prints schemaId, and each field as schemaId:fieldName
```

A field must exist as a schema field before it can be granted to an application — `applications create`/`policy update --field schemaId:fieldName` will fail with "policy metadata not found" for any field that was never declared via `schemas create` first.

### 5. Onboard the application-owning member (DIE)

```bash
./ondx members create --name "Department of Immigration and Emigration" --email die@die.gov.lk --phone "+1234567890" \
  --ou-id 01900000-0000-7000-8000-0000000000c1 --pb-url http://localhost:8083
# → prints memberId (dieMemberId)
```

### 6. Register DIE's application and grant it DRP's schema fields

The passport application needs the applicant's full name and permanent address to process an application:

```bash
./ondx applications create --name "Passport Application" --member-id <dieMemberId> \
  --field <schemaId>:person.fullName --field <schemaId>:person.permanentAddress \
  --pb-url http://localhost:8083
# → prints applicationId, plus the IdP client ID and secret (shown once)
```

Portal Backend creates the application's M2M client in DIE's ThunderID organization unit and grants it the `getdata` role. Store the printed secret: it can't be retrieved again. DIE then calls the Orchestration Engine with a `client_credentials` token for that client (`scope=getdata`, `resource=http://api.openndx.local`).

`applications create` grants only the fields passed at creation time. Use `policy update` to add or refresh grants later, passing every field the application's saved policy should contain. For example, to additionally grant `person.profession` while keeping the two fields already granted:

```bash
./ondx policy update --app-id <applicationId> \
  --field <schemaId>:person.fullName --field <schemaId>:person.permanentAddress --field <schemaId>:person.profession \
  --pb-url http://localhost:8083
```

## Commands

### `ondx login`

Browser-based OAuth2 Authorization Code + PKCE login (RFC 8252). Caches the resulting token.

| Flag                 | Env var         | Description                                                                                                                                                                                                                                                                    |
|----------------------|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--issuer`           | `NDX_ISSUER`    | Identity provider issuer/base URL; `--auth-url`/`--token-url` are discovered from `{issuer}/.well-known/openid-configuration` when they're not set explicitly                                                                                                                  |
| `--auth-url`         | `NDX_AUTH_URL`  | Identity provider authorization endpoint. Required unless `--issuer` supports discovery; overrides discovery when both are set                                                                                                                                                 |
| `--token-url`        | `NDX_TOKEN_URL` | Identity provider token endpoint. Required unless `--issuer` supports discovery; overrides discovery when both are set                                                                                                                                                         |
| `--client-id`        | `NDX_CLIENT_ID` | OAuth2 public client ID (required)                                                                                                                                                                                                                                             |
| `--scopes`           | `NDX_SCOPES`    | Space-separated scopes to request                                                                                                                                                                                                                                              |
| `--extra key=value`  | —               | Extra authorization *and* token-request query param, repeatable — e.g. `resource=...`, which overrides the profile's `resource` (see [ThunderID resource binding](#thunderid-resource-binding)). Persisted with the cached token, so it's reused automatically on refresh too. |
| `--callback-port`    | —               | Fixed local port for the redirect callback (0 = OS-assigned random port; see [Callback port](#callback-port))                                                                                                                                                                  |
| `--no-browser`       | —               | Print the login URL instead of opening a browser                                                                                                                                                                                                                               |
| `--insecure`         | —               | Skip TLS certificate verification (local dev only — see [TLS](#tls-and---insecure))                                                                                                                                                                                            |
| `--credentials-path` | —               | Where to cache the token (default `~/.openndx/credentials.json`, or `credentials-<profile>.json` for a non-`local` profile)                                                                                                                                                    |
| `--profile`          | `NDX_PROFILE`   | Named profile to source defaults from for this invocation (default: the config file's current profile) — see [Profiles](#ondx-profile-list--ondx-profile-use-name--ondx-profile-set-name-flags)                                                                                |

### `ondx members create`

Registers a member organization. `--name` and `--ou-id` are always required. Nothing is created in the IDP.

| Flag                          | Description                                                                                                                        |
|-------------------------------|------------------------------------------------------------------------------------------------------------------------------------|
| `--ou-id`                     | ID of the ThunderID organization unit this member maps to. Create it (and its users) in ThunderID first. Unique and not updatable. |
| `--email`                     | Optional organization contact email, e.g. a shared mailbox rather than a person's address. Not unique.                             |
| `--phone`                     | Optional organization contact phone number                                                                                         |
| `--pb-url` (env `NDX_PB_URL`) | Portal Backend base URL                                                                                                            |

### `ondx members list [--ou-id ...]`

Lists the members you can see: every member for an admin (optionally filtered to one organization unit's with `--ou-id`), or only your own organization unit's member otherwise. Add `--json` for the raw response.

### `ondx members get --member-id ...`

Shows one member's details. Members can only fetch their own organization unit's member. Add `--json` for the raw response.

### `ondx schemas create`

Registers a schema and its grantable fields directly, skipping GraphQL SDL/directive parsing entirely. `--name`, `--endpoint`, `--member-id`, and at least one `--field fieldName:accessControlType:source` are always required.

| Flag                                         | Description                                                                                                                                                                                                                                               |
|----------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--field fieldName:accessControlType:source` | One field per flag, repeatable. `accessControlType` is `public` or `restricted`; `source` is `primary` or `fallback`. Note this is a **different 3-part format** from `applications create`/`policy update`'s 2-part `schemaId:fieldName`.                |
| `--schema-id`                                | Optional schema ID, e.g. `drp-schema-v1`: 3-64 lowercase letters, digits, `.`, `_` or `-`. Use the ID the Orchestration Engine's `@sourceInfo(schemaId: ...)` names. Defaults to a generated UUID; can't be changed later, and a taken ID fails with 409. |
| `--description`                              | Schema description                                                                                                                                                                                                                                        |
| `--pb-url` (env `NDX_PB_URL`)                | Portal Backend base URL                                                                                                                                                                                                                                   |

Prints each field as `schemaId:fieldName` — the `--field` shape `applications create`/`policy update` expect, so you can copy straight from here.

### `ondx schemas list [--member-id ...]`

Lists the schemas you can see: every schema for an admin (optionally filtered to one member's with `--member-id`), or only your own member's schemas otherwise. Add `--json` for the raw response.

### `ondx schemas get --schema-id ...`

Shows one schema's details and its grantable fields, each printed as `schemaId:fieldName` — the same shape `applications create`/`policy update --field` expect. Members can only fetch their own member's schemas. Add `--json` for the schema and its fields as JSON.

### `ondx applications create`

Registers an application. `--name`, `--member-id`, and at least one `--field schemaId:fieldName` are always required.

| Flag                                      | Description                                                                                                                                                                                            |
|-------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--description`                           | Application description                                                                                                                                                                                |
| `--idp-application-id`, `--idp-client-id` | An OAuth2 client already provisioned in the IDP — must be given together. If both omitted, Portal Backend provisions the client itself, in the member's organization unit, and prints its secret once. |
| `--pb-url` (env `NDX_PB_URL`)             | Portal Backend base URL                                                                                                                                                                                |

### `ondx applications list [--member-id ...]`

Lists applications, optionally filtered to one member's. Add `--json` for the raw response.

### `ondx applications get --app-id ...`

Shows one application's details and current policy (its granted fields, printed as `schemaId:fieldName` — the same shape `--field` expects, so you can copy straight from here into `policy update`). Add `--json` for the raw response.

### `ondx policy update --app-id ... --field schemaId:fieldName ...`

Updates an application's saved policy and adds or refreshes the supplied PDP grants.
Pass every field the saved policy should contain, including ones it already has;
Portal Backend replaces its stored selected-field list. **Omitting a field does not
currently revoke its existing PDP grant.** Run `applications get` first to inspect
the saved list; it is not an authoritative check that omitted grants were revoked
in PDP.

| Flag                          | Description                                    |
|-------------------------------|------------------------------------------------|
| `--grant-duration`            | e.g. `30d` or `365d` (default: server default) |
| `--pb-url` (env `NDX_PB_URL`) | Portal Backend base URL                        |

Every command above also accepts `--credentials-path` and `--insecure`.

### `ondx profile list` / `ondx profile use <name>` / `ondx profile set <name> [flags]`

Every command above also accepts `--profile <name>` (env `NDX_PROFILE`) to source its flag defaults — `--issuer`/`--auth-url`/`--token-url`/`--client-id`/`--scopes`/`--callback-port`/`--resource` (login only), and `--pb-url`/`--insecure` (every command) — from a named profile instead of retyping them each time. Precedence is: explicit flag > `--profile`/`NDX_PROFILE` for that one invocation > the config file's current profile > (nothing).

Profiles are stored in `~/.openndx/config.json`. A built-in `local` profile (see [Log in](#2-log-in)) always exists even before that file does, so `ondx login` works with zero setup; anything you `ondx profile set local ...` overrides it.

```bash
./ondx profile set staging --issuer https://idp.staging.example.com --client-id ondx-cli-staging --resource https://pb.staging.example.com --pb-url https://pb.staging.example.com
./ondx profile use staging       # makes staging the default for every subsequent command
./ondx profile list               # * marks the current profile
./ondx login --profile local      # one-off override back to local without switching the default
```

`profile set` only changes the flags you pass — omitted flags keep their existing value in that profile. Each non-`local` profile also gets its own credentials cache (`~/.openndx/credentials-<name>.json`) so switching profiles can't pick up a token cached against a different identity provider.

### `ondx version`

Prints the release version, commit, and build date (injected by GoReleaser at release time), plus the Go version and OS/arch. Builds from `go install`/`go build` report the module version and commit Go embeds instead. `ondx --version` is an alias.

## Notes

### TLS and `--insecure`

ThunderID's local-dev instance serves plain HTTP on `http://localhost:8090`, so no TLS is involved and `--insecure` has no effect against it (the built-in `local` profile still sets it, which is harmless over HTTP). `--insecure` skips TLS certificate verification for HTTPS endpoints with self-signed or otherwise untrusted certificates. It is safe only for local/test setups — never pass it against a real deployment.

### Callback port

`ondx login` opens a short-lived local HTTP server to receive the OAuth2 redirect. RFC 8252 recommends a random OS-assigned port (the default here, `--callback-port 0`) since a fixed port can collide with another process or a previous unclean exit. Use a fixed `--callback-port` only when the identity provider's registered redirect URI requires an exact match rather than a wildcard port — which is the case for ThunderID's `NDX_CLI` client today (`thunderid/bootstrap/application.yaml`, pinned to `http://127.0.0.1:8765/callback`). Logging in with `--client-id NDX_CLI` enforces this: `ondx login` rejects any `--callback-port` other than `8765` for that client (including the `0` default above) instead of attempting a login ThunderID would reject anyway.

### ThunderID resource binding

ThunderID sets an access token's `aud` claim to the resource server requested with a `resource=` parameter. Portal Backend only accepts tokens for its own audience (`IDP_AUDIENCE`, `http://pb.openndx.local` locally, the PB resource server in `thunderid/bootstrap/resource.yaml`), whichever client requested them. So `ondx login` has to request that resource:

- The built-in `local` profile already does (`resource: http://pb.openndx.local`), so a bare `ondx login` works.
- For another environment, set it once on the profile: `ondx profile set <name> --resource <pb-audience>`.
- `--extra resource=...` on a single login overrides the profile.

If you saved a `local` profile in `~/.openndx/config.json` before it had a `resource`, run `ondx profile set local --resource http://pb.openndx.local`, then log in again. Tokens cached before that are for the `NDX_CLI` audience and Portal Backend rejects them with 401.

### Limitations

- **An application's client secret is shown only once**, in the `applications create` output. Portal Backend doesn't store it and ThunderID doesn't return it again. Applications created by approving an application submission don't surface it at all yet.
- **Members need their organization unit created in ThunderID first.** Portal Backend only stores the link (`--ou-id`); it does not create organization units or users. The local stack seeds `drp` (`01900000-0000-7000-8000-0000000000c0`) and `die` (`01900000-0000-7000-8000-0000000000c1`), with staff users `drp-admin` / `die-admin` (password `1234`) who get the `OpenNDX_Member` role and an `ouId` claim.
- **No delete or update commands yet.** Only `members create`/`list`/`get`, `schemas create`/`list`/`get`, `applications create`/`list`/`get`, and `policy update` are implemented.
- **`ondx applications get`/`list` output for `--field` values always reflects the full current policy** — there's no "show me the diff" helper; compare manually before running `policy update`.

## Development

```bash
go build ./cmd/ondx/... ./internal/cli/...
go vet ./cmd/ondx/... ./internal/cli/...
go test ./internal/cli/...
```

### Releases

Pushing a `v*.*.*` tag runs `.github/workflows/release.yml`, whose `goreleaser` job builds `ondx` for linux/darwin/windows × amd64/arm64 per [`.goreleaser.yaml`](../../.goreleaser.yaml), creates the GitHub Release with the archives and `checksums.txt`, attests their build provenance, and updates the `openndx/homebrew-tap` cask (skipped for prereleases or when the `TAP_TOKEN` secret isn't set; the token needs write access to the tap repo). Dry-run it locally with `goreleaser release --snapshot --clean` (output in `dist/`).

### Project Structure

```
cmd/ondx/
├── main.go                 # Entry point: flag parsing and subcommand dispatch
└── version.go              # `ondx version`; version/commit/date injected via -ldflags at release

internal/cli/
├── auth/                    # PKCE, browser-based login flow, token cache/refresh
├── pbclient/                # Portal Backend API client (reuses internal/pb member/schema/application types)
└── profile/                 # Named profiles (issuer/client-id/scopes/resource/pb-url/...) cached at ~/.openndx/config.json
```
