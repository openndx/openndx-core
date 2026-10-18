# Portals

The OpenNDX web apps, kept in one pnpm workspace. This page covers what they share: tooling, runtime configuration, ports and Docker. Each app's README covers the app itself.

## Overview

| App | Users | Talks to | In Compose |
|---|---|---|---|
| [Consent Portal](apps/consent/README.md) (`consent-portal`) | Citizens approving or revoking access to their data | Consent Engine, identity provider | yes, port 5173 |
| [Admin Portal](apps/admin/README.md) (`admin-portal`) | Operators managing members and reviewing submissions | Portal Backend, audit service, identity provider | no |
| [Member Portal](apps/member/README.md) (`member-portal`) | Staff of member organizations registering schemas and applications | Portal Backend, audit service, identity provider | no |

All three are React 19, TypeScript, Vite and Tailwind apps served in Docker by nginx. The consent portal is current. The admin and member portals are documented as they are in the code, and each README lists the gaps that stop them working against the current Portal Backend.

```
portals/
├── apps/
│   ├── admin/
│   ├── consent/
│   └── member/
├── packages/            # shared code; only a README so far
├── package.json         # Node >= 24, pnpm 11.1.3 (packageManager)
├── pnpm-workspace.yaml  # apps/* and packages/*
└── setup-portals.sh
```

## Quick start

Node 24+ and pnpm. The pinned pnpm version is picked up by `corepack enable`.

```bash
cd portals
pnpm install                               # once, for the whole workspace
pnpm --filter consent-portal run dev       # or admin-portal, member-portal
```

There are no root scripts. Run everything with `pnpm --filter <app> run <script>`. Every app has `dev`, `build` (`tsc -b && vite build`), `lint` and `preview`. None has a test script.

To run the consent portal with its backends, use Docker Compose from the repo root: `docker compose up --build consent-portal`.

## Configuration

The apps are configured at **runtime**, so one build serves any environment. Each `index.html` loads `config.js` before the app, and the app reads `window.configs`:

- **Locally**, create `apps/<app>/public/config.js` (git-ignored). Only the consent portal ships a template, `config.example.js`.
- **In Docker**, `entrypoint.sh` writes `config.js` when the container starts, from environment variables.
- A `.env` file does **not** fill `window.configs`. In the admin and member apps it sets only `VITE_PORT` and `VITE_BASE_PATH`, which Vite reads at dev and build time. The consent app reads neither.

The three apps use three different naming conventions. Use the names in this table:

| App | `window.configs` keys | Docker environment variables | Scope list format |
|---|---|---|---|
| Consent | `consentEngineUrl`, `idpBaseUrl`, `idpClientId`, `idpScope`, `idpSignInRedirectUrl`, `idpSignOutRedirectUrl` | `CONSENT_ENGINE_URL`, `IDP_BASE_URL`, `IDP_CLIENT_ID`, `IDP_SCOPE`, `IDP_SIGN_IN_REDIRECT_URL`, `IDP_SIGN_OUT_REDIRECT_URL` | space-separated |
| Admin | `VITE_API_URL`, `VITE_LOGS_URL`, `VITE_IDP_CLIENT_ID`, `VITE_IDP_BASE_URL`, `VITE_IDP_SCOPE`, `VITE_IDP_ADMIN_ROLE`, `VITE_SIGN_IN_REDIRECT_URL`, `VITE_SIGN_OUT_REDIRECT_URL` | the same names | comma-separated |
| Member | `API_URL`, `LOGS_URL`, `CLIENT_ID`, `BASE_URL`, `SCOPE`, `SIGN_IN_REDIRECT_URL`, `SIGN_OUT_REDIRECT_URL` | each key with a `VITE_` prefix (`VITE_API_URL`, ...) | comma-separated |

To check that a config loaded, open the portal, then in the browser console inspect `window.configs`, and confirm `config.js` returns `200` in the Network tab.

`setup-portals.sh` writes placeholder `config.js` files for all three apps. The values are test values aimed at an Asgardeo-style tenant, and the consent file's `consentEngineUrl` points at the Portal Backend port. It prints per-app ports (5174, 5175, 5176) but leaves every redirect URL on 5173, and the consent app ignores `VITE_PORT`. Do not use it with the Compose stack; write the config by hand as in the app READMEs.

## Ports

| Where | Port |
|---|---|
| Vite dev server, every app | 5173 by default |
| Admin and member | `VITE_PORT` changes it |
| Consent | `-- --port <n>` only: `pnpm --filter consent-portal run dev -- --port 5180` |
| Docker image | nginx on 80 (`GET /health` answers) |
| Compose `consent-portal` | host 5173 to container 80 |

All apps default to the same port, so run one at a time or change the port and register the new origin (redirect URL and CORS) with the identity provider.

## Docker

An image is built from the workspace root, because its `COPY` lines read `package.json`, `pnpm-workspace.yaml` and the lockfile from `portals/`:

```bash
cd portals
docker build -f apps/consent/Dockerfile -t consent-portal .     # or apps/admin, apps/member
```

Compose and CI both use `portals/` as the build context. `make validate-docker-build <portal>` builds from inside the app directory instead, where those files are missing, so it does not work for the portals; use the command above.

## Development

From the repo root, `make setup`, `validate-build`, `check-lint`, `quality-check` and `run` accept `consent-portal`, `admin-portal` and `member-portal`:

```bash
make run consent-portal             # pnpm run dev in apps/consent
make validate-build admin-portal    # build
make check-lint member-portal
```

`make validate-test <portal>` runs the TypeScript check and lint, not tests. `format`, `lint`, `staticcheck` and `security` are Go-only.

`packages/` is empty, so each app carries its own copy of the `window.configs` typing, the audit log service, the schema and application types, the logo, and the nginx and entrypoint files. See [packages/README.md](packages/README.md).

## More docs

- [Consent Portal](apps/consent/README.md), [Admin Portal](apps/admin/README.md), [Member Portal](apps/member/README.md)
- [Portal Backend](../cmd/pb/README.md) and [Consent Engine](../cmd/ce/README.md): the backends the portals call
