# Portal Backend

A secure Go-based REST API for managing data exchange workflows, including member management, schema submissions, and application processing.

## Overview

The Portal Backend provides REST APIs for the Admin Portal and Member Portal, handling authentication, authorization, and business logic for data exchange operations.

## Features

- **JWT Authentication** with Asgardeo integration
- **Role-Based Access Control (RBAC)** with granular permissions
- **PostgreSQL Database** with automatic schema management
- **Thread-Safe Caching** for optimal performance
- **OpenAPI Documentation** at `/openapi.yaml`
- **Comprehensive Health Monitoring**
- **Docker Support** for containerized deployment
- **Audit Logging** for compliance

## Quick Start

### Prerequisites

- Go 1.26+
- PostgreSQL 13+
- Docker (optional)

### 1. Environment Setup

> [!NOTE]
> The Portal Backend no longer auto-loads a `.env` file at startup — variables must come from the process environment. Copy `cmd/pb/.env.example` to `cmd/pb/.env`, update it, then source it manually before running the service (see step 2).

Copy `cmd/pb/.env.example` to `cmd/pb/.env` and update it:

```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USERNAME=postgres
DB_PASSWORD=your_password
DB_NAME=portal_backend
DB_SSLMODE=disable
RUN_MIGRATION=true

# JWT Authentication (Required)
IDP_BASE_URL=https://api.asgardeo.io/t/your-org
IDP_MEMBER_PORTAL_CLIENT_ID=your_member_client_id
IDP_ADMIN_PORTAL_CLIENT_ID=your_admin_client_id

# Policy Decision Point
PDP_SERVICE_URL=http://localhost:8082


# Optional: IDP Management (for member creation)
IDP_CLIENT_ID=management_client_id
IDP_CLIENT_SECRET=management_client_secret
IDP_SCOPE="internal_user_mgt_create internal_user_mgt_list"
```

### 2. Run the Service

```bash
# Load cmd/pb/.env into the shell environment (from the repo root)
set -a
source cmd/pb/.env
set +a

# Run the server
go run ./cmd/pb

# Or build and run
go build -o pb ./cmd/pb && ./pb
```

The service runs on port 8083 by default.

## Configuration

### Server Configuration

```bash
PORT=8083                         # Server port (default: 8083)
CORS_ALLOWED_ORIGINS=*            # CORS allowed origins
```

## API Endpoints

### Core Resources

- **Members** - `/api/v1/members` - User profile and membership management
- **Schemas** - `/api/v1/schemas` - Data schema definitions and management
- **Schema Submissions** - `/api/v1/schema-submissions` - Schema submission workflow
- **Applications** - `/api/v1/applications` - Application definitions
- **Application Submissions** - `/api/v1/application-submissions` - Application submission workflow

### System Endpoints

- **Health Check** - `/health` - System health and database status
- **API Documentation** - `/openapi.yaml` - OpenAPI specification

### Authentication & Authorization

**Supported Roles:**
- `OpenNDX_Admin` - Full system access
- `OpenNDX_Member` - Standard user access to own resources
- `OpenNDX_System` - System-level read access

**JWT Requirements:**
- Issuer: Asgardeo identity provider
- Audience: Configured client IDs (member-portal, admin-portal)
- Claims: Valid roles and user information
- Validation: JWKS-based signature verification

## Testing

### Run Tests

```bash
# Unit tests only (from the repo root)
go test ./cmd/pb/... ./internal/pb/...

# Integration tests with PostgreSQL
make test-postgres

# Tests with race detection
go test -race ./cmd/pb/... ./internal/pb/...

# Coverage report
go test -coverprofile=coverage.out ./cmd/pb/... ./internal/pb/...
go tool cover -html=coverage.out
```

### Test Database Setup

```bash
export TEST_DB_PASSWORD=test_password
make test-local
```

## Architecture

### Project Structure

```
cmd/pb/
└── main.go                 # Application entry point

internal/pb/
├── v1/                     # API version 1
│   ├── handlers/           # HTTP request handlers
│   ├── middleware/         # Authentication & authorization
│   ├── models/            # Data models and DTOs
│   ├── services/          # Business logic layer
│   └── utils/             # Utility functions
├── shared/                # Shared utilities
└── idp/                   # Identity provider integrations
```

### Security Architecture

```
Request → CORS → JWT Validation → Authorization → Resource Access
    ↓        ↓           ↓              ↓             ↓
 Origin   Token      Role Check    Permission    Ownership
 Check    Verify     & Claims      Validation    Validation
```

### Database Schema

**Core Tables:**
- `members` - User profiles and membership information
- `schemas` - Data schema definitions with versioning
- `schema_submissions` - Schema submission workflow and status
- `applications` - Application templates and definitions
- `application_submissions` - Application submission workflow

**Features:**
- Auto-migration on startup
- Connection pooling with configurable limits
- Health monitoring with metrics
- Transaction support with timeouts

## Health Check

`GET /health` returns comprehensive system status:

```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T12:00:00Z",
  "database": {
    "status": "connected",
    "open_connections": 5,
    "max_open_connections": 25
  }
}
```

## Docker

```bash
# Build image (from the repo root)
docker build -t portal-backend -f cmd/pb/Dockerfile .

# Run container
docker run -p 8083:8083 \
  -e DB_HOST=host.docker.internal \
  -e PDP_SERVICE_URL=http://host.docker.internal:8082 \
  --env-file cmd/pb/.env \
  portal-backend
```
