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

- Go 1.21+
- PostgreSQL 13+
- Docker (optional)

### 1. Environment Setup

Create a `.env` file:

```bash
# Database Configuration
CHOREO_DB_portal_backend_HOSTNAME=localhost
CHOREO_DB_portal_backend_PORT=5432
CHOREO_DB_portal_backend_USERNAME=postgres
CHOREO_DB_portal_backend_PASSWORD=your_password
CHOREO_DB_portal_backend_DATABASENAME=portal_backend

# JWT Authentication (Required)
IDP_BASE_URL=https://api.asgardeo.io/t/your-org
IDP_MEMBER_PORTAL_CLIENT_ID=your_member_client_id
IDP_ADMIN_PORTAL_CLIENT_ID=your_admin_client_id

# Policy Decision Point
CHOREO_PDP_CONNECTION_SERVICEURL=http://localhost:8082
CHOREO_PDP_CONNECTION_CHOREOAPIKEY=your_pdp_key

# Optional: Asgardeo Management (for member creation)
IDP_CLIENT_ID=management_client_id
IDP_CLIENT_SECRET=management_client_secret
IDP_SCOPES="internal_user_mgt_create internal_user_mgt_list"
```

### 2. Run the Service

```bash
# Install dependencies
go mod download

# Run the server
go run main.go

# Or build and run
go build -o portal-backend
./portal-backend
```

The service runs on port 3000 by default.

## Configuration

### Database Configuration

```bash
DB_MAX_OPEN_CONNS=25              # Maximum open connections
DB_MAX_IDLE_CONNS=5               # Maximum idle connections
DB_CONN_MAX_LIFETIME=1h           # Connection maximum lifetime
DB_QUERY_TIMEOUT=30s              # Query timeout duration
```

### JWT Security

```bash
JWT_VALIDATION_STRICT=true        # Strict JWT validation mode
JWT_CACHE_DURATION=15m            # JWKS cache duration
JWT_TIMEOUT=10s                   # Token validation timeout
```

### Server Configuration

```bash
PORT=3000                         # Server port (default: 3000)
LOG_LEVEL=info                    # Logging level (debug, info, warn, error)
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
- `OpenDIF_Admin` - Full system access
- `OpenDIF_Member` - Standard user access to own resources
- `OpenDIF_System` - System-level read access

**JWT Requirements:**
- Issuer: Asgardeo identity provider
- Audience: Configured client IDs (member-portal, admin-portal)
- Claims: Valid roles and user information
- Validation: JWKS-based signature verification

## Testing

### Run Tests

```bash
# Unit tests only
go test ./...

# Integration tests with PostgreSQL
make test-postgres

# Tests with race detection
go test -race ./...

# Coverage report
go test -coverprofile=coverage.out ./...
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
portal-backend/
├── main.go                 # Application entry point
├── v1/                     # API version 1
│   ├── handlers/           # HTTP request handlers
│   ├── middleware/         # Authentication & authorization
│   ├── models/            # Data models and DTOs
│   ├── services/          # Business logic layer
│   └── utils/             # Utility functions
├── shared/                # Shared utilities
├── idp/                   # Identity provider integrations
└── middleware/            # Global middleware
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
# Build image
docker build -t portal-backend .

# Run container
docker run -p 3000:3000 \
  -e CHOREO_DB_portal_backend_HOSTNAME=host.docker.internal \
  --env-file .env \
  portal-backend
```
