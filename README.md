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
    - Copy `.env.example` to `.env` in each service directory.
    - Update the `.env` files with your IdP configuration (Client IDs, Issuer URLs, etc.) and database credentials.

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

3. **Setup Frontend Services** - Installs npm dependencies (`npm ci`) for:
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

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](docs/contributing/) for details on:
- Development setup
- Pull request process
- Reporting issues

## Security

For security concerns, please see our [Security Policy](SECURITY.md). **Do not report security vulnerabilities through public GitHub issues.**

## Documentation

For detailed documentation, see the `docs/` directory.
