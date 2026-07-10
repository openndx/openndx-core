# Policy Decision Point (PDP)

Authorization service using Open Policy Agent (OPA) that evaluates data access requests and determines consent requirements.

## Overview

The PDP provides attribute-based access control (ABAC) with field-level permissions. It uses Open Policy Agent (OPA) v1 with Rego v1 policies and stores policy metadata in PostgreSQL for real-time evaluation.

**Technology**: Go + Open Policy Agent (OPA) v1 + Rego v1 + PostgreSQL  
**Port**: 8082

## Features

- **Real-time Policy Evaluation** - Policies loaded from database on startup
- **Field-level Access Control** - Granular permissions for individual data fields
- **Consent Management** - Automatic consent requirement calculation
- **Allow List Management** - Dynamic application authorization for restricted fields
- **OPA v1 Integration** - Modern Open Policy Agent with Rego v1 syntax
- **Database-driven** - Policy metadata stored in PostgreSQL

## Quick Start

### Prerequisites

- Go 1.24+
- PostgreSQL 13+

### Run the Service

```bash
# Install dependencies
go mod download

# Copy environment template
cp .env.example .env

# Edit .env with your database configuration
# DB_HOST=localhost
# DB_PORT=5432
# DB_USERNAME=postgres
# DB_PASSWORD=password
# DB_NAME=pdp

# Run locally
go run main.go

# Or build and run
go build -o policy-decision-point
./policy-decision-point
```

The service runs on port 8082 by default.

## Configuration

### Environment Variables

All configuration is done via environment variables. See `.env.example` for a complete list.

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Service port | `8082` |
| `ENVIRONMENT` | `production` or `local` | `local` |
| `IDP_ORG_NAME` | IDP organization name | - |
| `IDP_ISSUER` | JWT issuer URL | - |
| `IDP_AUDIENCE` | JWT audience | - |
| `IDP_JWKS_URL` | JWKS endpoint URL | - |
| `DB_HOST` | Database host | `localhost` |
| `DB_PORT` | Database port | `5432` |
| `DB_USERNAME` | Database username | `postgres` |
| `DB_PASSWORD` | Database password | - |
| `DB_NAME` | Database name | `pdp` |
| `DB_SSLMODE` | SSL mode | `require` |

**Optional:**
```bash
RUN_MIGRATION=false       # Set to "true" to run migrations on startup
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/policy/decide` | POST | Authorization decision |
| `/api/v1/policy/metadata` | POST | Create policy metadata for fields |
| `/api/v1/policy/update-allowlist` | POST | Update allow list for applications |
| `/health` | GET | Health check |
| `/debug` | GET | Debug information |
| `/debug/db` | GET | Database connection status |

### Authorization Request

**Endpoint:** `POST /api/v1/policy/decide`

**Request:**
```json
{
  "applicationId": "passport-app",
  "requiredFields": [
    {
      "fieldName": "person.fullName",
      "schemaId": "schema-123"
    },
    {
      "fieldName": "person.photo",
      "schemaId": "schema-123"
    }
  ]
}
```

**Response:**
```json
{
  "appAuthorized": true,
  "unauthorizedFields": [],
  "appAccessExpired": false,
  "expiredFields": [],
  "appRequiresOwnerConsent": true,
  "consentRequiredFields": [
    {
      "fieldName": "person.photo",
      "schemaId": "schema-123",
      "displayName": "Photo",
      "description": "Person's photo"
    }
  ]
}
```

### Policy Metadata Management

**Create Policy Metadata:** `POST /api/v1/policy/metadata`

```json
{
  "schema_id": "schema-123",
  "records": [
    {
      "field_name": "person.fullName",
      "display_name": "Full Name",
      "access_control_type": "public"
    }
  ]
}
```

**Update Allow List:** `POST /api/v1/policy/update-allowlist`

```json
{
  "application_id": "passport-app",
  "records": [
    {
      "field_name": "person.fullName",
      "schema_id": "schema-123"
    }
  ],
  "grant_duration": "ONE_MONTH"
}
```

## Access Control Logic

### Field Types

1. **Public Fields** (`access_control_type: "public"`)
   - Any app can access
   - Consent required only if `consent_required: true`

2. **Restricted Fields** (`access_control_type: "restricted"`)
   - Only apps in `allow_list` can access
   - Consent required if `consent_required: true`

### Decision Logic

- **Allow**: All requested fields are authorized for the app
- **Deny**: Any requested field is not authorized for the app
- **Consent Required**: Any requested field has `consent_required: true`

### Consent Logic

Consent requirement is calculated as: `!is_owner && access_control_type != "public"`

- **Owner Fields** (`is_owner: true`): No consent required
- **Public Fields**: No consent required for non-owners
- **Restricted Fields**: Consent required for non-owners

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -cover

# Test authorization scenarios
curl -X POST http://localhost:8082/api/v1/policy/decide \
  -H "Content-Type: application/json" \
  -d '{
    "applicationId": "passport-app",
    "requiredFields": [
      {
        "fieldName": "person.fullName",
        "schemaId": "schema-123"
      }
    ]
  }'
```

## Architecture

### Database Schema

**`policy_metadata` Table:**
- `id` (UUID) - Primary key
- `schema_id` (TEXT) - Schema identifier
- `field_name` (TEXT) - Data field name
- `display_name` (TEXT) - Human-readable name
- `access_control_type` (ENUM) - public/restricted
- `is_owner` (BOOLEAN) - Field ownership flag
- `allow_list` (JSONB) - Authorized applications with expiration
- `created_at`, `updated_at` (TIMESTAMP)

### Policy Evaluation Flow

```
Request → Load Policy Metadata → OPA Evaluation → Consent Check → Decision
```

## Health Check

```bash
curl http://localhost:8082/health
```

**Response:**
```json
{
  "service": "policy-decision-point",
  "status": "healthy"
}
```
