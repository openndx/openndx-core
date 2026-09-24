# OpenNDX

A comprehensive data exchange platform consisting of multiple microservices and portals for secure data sharing and consent management.

## Architecture

### Backend Services (Go)

- **Orchestration Engine** - Data exchange workflow orchestration
- **Policy Decision Point** - Policy enforcement
- **Consent Engine** - User consent management and validation
- **Portal Backend** - Backend service for the `Admin Portal` and the `Member Portal`

### Frontend Portals (React/TypeScript)

- **Member Portal** - Management of `Data sources` or `Applications` by `OpenNDX Members`
- **Admin Portal** - Administrative dashboard for the `OpenNDX Admins`
- **Consent Portal** - Citizen-facing interface for data consent

### Optional Components

- **Observability Stack** (`observability/`) - Metrics collection and visualization (Prometheus, Grafana)
- **Audit Service** ([LSFLK/argus](https://github.com/LSFLK/argus)) - Audit logging and event tracking (optional, services function normally without it)

### Services

| Service                         | Port | Purpose                          | Documentation                   |
|---------------------------------|------|----------------------------------|---------------------------------|
| **Orchestration Engine (OE)**   | 4000 | Request coordination and routing | [OE README](cmd/oe/README.md)   |
| **Policy Decision Point (PDP)** | 8082 | ABAC authorization using OPA     | [PDP README](cmd/pdp/README.md) |
| **Consent Engine (CE)**         | 8081 | Consent management and workflow  | [CE README](cmd/ce/README.md)   |
| **Portal Backend (PB)**         | -    | Backend for Admin/Member Portals | [PB README](cmd/pb/README.md)   |

### Request Flow

```text
Data Consumer → Orchestration Engine → Policy Decision Point (PDP)
                     ↓
              Consent Engine (CE) ← (if consent required)
                     ↓
              Data Provider
```

1. **Data Consumer Request** — a GraphQL request arrives at the Orchestration Engine.
2. **PDP Evaluation** — the Orchestration Engine asks the PDP whether the request is
   authorized and which fields (if any) still require consent.
3. **Consent Management** — if consent is required, the Orchestration Engine calls
   the Consent Engine, which returns a consent portal URL for the data owner.
4. **Data Access** — once authorized (and consented, if required), the Orchestration
   Engine fetches data from the appropriate Data Provider.

See the [Orchestration Engine README](cmd/oe/README.md) for full request/response
payloads and the [GraphQL/Policy/Consent flow doc](docs/FLOW-graphql-policy-consent-and-esignet-gap.md)
for a deeper walkthrough.

## How to Deploy

### Prerequisites

Before deploying OpenNDX, you must configure an Identity Provider (IdP) to handle authentication and authorization.

1.  **Configure IdP**: Set up an IdP (e.g., Asgardeo, Keycloak, Auth0) to manage users and roles.
2.  **Create Users**: Create the necessary users in your IdP.
3.  **Assign Roles**:
    - Create a role named `openndx-admin`.
    - Assign this role to users who require administrative access to the OpenNDX Admin Portal.
    - Ensure other roles (e.g., `openndx-member`) are created and assigned as needed for Member Portal access.

### Deployment Steps

1.  **Clone the Repository**:
    ```bash
    git clone https://github.com/OpenNDX/openndx-core.git
    cd openndx-core
    ```

2.  **Configure Environment**:
    - Optional: the root `compose.yml` defaults every variable to a working local-dev value, including the bundled ThunderID IdP.
    - To override any of them, copy the root `.env.example` to `.env` (`cp .env.example .env`) and edit only what you need.

3.  **Build and Run**:
    - Use the provided Makefile to build and run services.
    ```bash
    make setup-all
    make validate-build-all
    make run-all # If available, or run services individually
    ```


## Quick Start

### Initial Setup

```bash
make setup-all
```

This command will:

1. **Install Git Hooks** - Sets up pre-commit hooks that automatically run quality checks, build validation, and tests for services with staged changes
2. **Setup Go Services** - Installs dependencies (`go mod tidy` and `go mod download`) for:

   - orchestration-engine
   - policy-decision-point
   - consent-engine
   - portal-backend

3. **Setup Frontend Services** - Installs pnpm dependencies (`pnpm install --frozen-lockfile`) for:
   - member-portal
   - admin-portal
   - consent-portal

### Build and Run

```bash
# Build all services
make validate-build-all

# Run a specific service
make run <service-name>
```

## Available Commands

```bash
make help                    # Show all available commands
make setup <service>         # Setup a specific service
make validate-build <service> # Build and validate a service
make validate-test <service>  # Run tests for a service
make quality-check <service>  # Run code quality checks
```

## Local Development Stack (Docker Compose)

The repo root `compose.yml` brings up Postgres, ThunderID (IdP), OE, PDP, CE,
Portal Backend, the consent portal, and the optional audit service — a
self-contained stack for exercising the full request flow end-to-end without
running each Go binary by hand.

```bash
docker compose up --build
```

Every variable has a working local-dev default, so no `.env` is needed. To
override any of them, `cp .env.example .env` and edit only what you need —
docker compose automatically loads a `.env` file from the working directory.

### Testing

```bash
# Unit tests (from the repo root)
go test ./cmd/pdp/... ./internal/pdp/... -v
go test ./cmd/ce/... ./internal/ce/... -v
go test ./cmd/oe/... ./internal/oe/... -v

# Integration tests
cd tests/integration && docker compose -f docker-compose.test.yml up -d && \
  (go test -v ./...; docker compose -f docker-compose.test.yml down -v)
```

### API Reference

| Service                      | Endpoint                         | Purpose                            |
|------------------------------|----------------------------------|------------------------------------|
| Orchestration Engine (4000)  | `POST /graphql`                  | GraphQL endpoint for data requests |
| Policy Decision Point (8082) | `POST /api/v1/policy/decide`     | Authorization decision             |
| Consent Engine (8081)        | `POST /internal/api/v1/consents` | Process consent workflow request   |
| Consent Engine (8081)        | `GET /consents/{id}`             | Get consent status                 |
| Consent Engine (8081)        | `PUT /consents/{id}`             | Update consent status              |
| Consent Engine (8081)        | `DELETE /consents/{id}`          | Revoke consent                     |
| Consent Engine (8081)        | `GET /data-owner/{owner}`        | Get consents by data owner         |
| Consent Engine (8081)        | `GET /consumer/{consumer}`       | Get consents by consumer           |
| all services                 | `GET /health`                    | Health check                       |

```bash
# Policy decision
curl -X POST http://localhost:8082/api/v1/policy/decide \
  -H "Content-Type: application/json" \
  -d '{
    "consumer_id": "passport-app",
    "app_id": "passport-app",
    "request_id": "req_123",
    "required_fields": ["person.fullName", "person.photo"]
  }'

# Consent request
curl -X POST http://localhost:8081/internal/api/v1/consents \
  -H "Content-Type: application/json" \
  -d '{
    "app_id": "passport-app",
    "data_fields": [
      {
        "owner_type": "citizen",
        "owner_id": "199512345678",
        "fields": ["person.permanentAddress"]
      }
    ],
    "purpose": "passport_application",
    "session_id": "session_123",
    "redirect_url": "https://passport-app.gov.lk/callback"
  }'
```

### Environment Variables

`.env.example` lists every variable `compose.yml` reads, grouped by service, at
its default value. The most common ones:

| Variable                                          | Local default                           | Production example   |
|---------------------------------------------------|-----------------------------------------|----------------------|
| `ENVIRONMENT`                                     | `local`                                 | `production`         |
| `PDP_PORT` / `CE_PORT` / `OE_PORT` / `PB_PORT`    | `8082` / `8081` / `4000` / `8083`       | as needed            |
| `DB_USER` / `DB_PASSWORD`                         | `postgres` / `1234`                     | real credentials     |
| `IDP_PUBLIC_URL`                                  | `http://localhost:8090`                 | your IdP's HTTPS URL |
| `BUILD_VERSION` / `BUILD_TIME` / `GIT_COMMIT`     | `dev` / empty / `local`                 | CI-provided values   |
| `OTEL_METRICS_EXPORTER`                           | `prometheus`                            | as needed            |
| `AUDIT_SERVICE_URL` / `ARGUS_API_KEY`             | `http://audit-service:3001` / `1234`    | as needed            |

All local-dev secrets default to `1234` — never reuse them outside local dev.

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](docs/contributing/) for details on:
- Development setup
- Pull request process
- Reporting issues

## Security

For security concerns, please see our [Security Policy](SECURITY.md). **Do not report security vulnerabilities through public GitHub issues.**

## Documentation

For detailed documentation, see the `docs/` directory.
