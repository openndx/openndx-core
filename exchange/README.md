# Exchange Services

Microservices-based data exchange platform with policy enforcement and consent management.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.21+ (for local development)

### Start Services

```bash
# Start all services
docker-compose up -d

# Run integration tests
cd ../tests/integration && docker compose -f docker-compose.test.yml up -d && go test -v ./... && docker compose -f docker-compose.test.yml down -v
```

## Services

| Service                         | Port | Purpose                          | Documentation                                 |
|---------------------------------|------|----------------------------------|-----------------------------------------------|
| **Policy Decision Point (PDP)** | 8082 | ABAC authorization using OPA     | [PDP README](policy-decision-point/README.md) |
| **Consent Engine (CE)**         | 8081 | Consent management and workflow  | [CE README](consent-engine/README.md)         |
| **Orchestration Engine (OE)**   | 8080 | Request coordination and routing | [OE README](orchestration-engine/README.md)   |

## Architecture

```
Data Consumer → Orchestration Engine → Policy Decision Point (PDP)
                     ↓
              Consent Engine (CE) ← (if consent required)
                     ↓
              Data Provider
```

### Key Features

- **ABAC Authorization**: Attribute-based access control with field-level permissions
- **Consent Management**: Complete consent workflow for data owners
- **GraphQL Schema Support**: Convert GraphQL SDL to provider metadata
- **Multi-Environment**: Local development and production deployment support
- **Docker Ready**: Containerized services with Docker Compose orchestration

## Data Flow

### 1. Data Consumer Request

The Data Consumer sends a GraphQL request to the Orchestration Engine:

```json
{
  "query": "query GetUser($id: ID!) { user(id: $id) { id name address profession }",
  "variables": { "id": "123" },
  "operationName": "GetUser"
}
```

### 2. Policy Decision Point (PDP) Evaluation

The Orchestration Engine forwards the request to the PDP for authorization:

**Request:**

```json
{
  "consumer_id": "passport-app",
  "app_id": "passport-app",
  "request_id": "req_123",
  "required_fields": ["person.fullName", "person.photo"]
}
```

**Response:**

```json
{
  "allow": true,
  "consent_required": true,
  "consent_required_fields": ["person.photo"]
}
```

### 3. Consent Management (if required)

When consent is required, the Orchestration Engine calls the Consent Engine:

**Request:**

```json
{
  "app_id": "passport-app",
  "data_fields": [
    {
      "owner_type": "citizen",
      "owner_id": "199512345678",
      "fields": ["person.photo"]
    }
  ],
  "purpose": "passport_application",
  "session_id": "session_123",
  "redirect_url": "https://passport-app.gov.lk/callback"
}
```

**Response:**

```json
{
  "id": "consent_abc123",
  "status": "pending",
  "consent_portal_url": "/consent-portal/consent_abc123",
  "expires_at": "2025-10-10T10:20:00Z"
}
```

### 4. Data Access

Once authorized (and consented if required), the Orchestration Engine proceeds to fetch data from the appropriate Data Provider.

## API Reference

### Policy Decision Point (Port 8082)

- `POST /decide` - Authorization decision
- `GET /health` - Health check

### Consent Engine (Port 8081)

- `POST /consent` - Process consent workflow request
- `GET /consents/{id}` - Get consent status
- `PUT /consents/{id}` - Update consent status
- `DELETE /consents/{id}` - Revoke consent
- `GET /data-owner/{owner}` - Get consents by data owner
- `GET /consumer/{consumer}` - Get consents by consumer
- `GET /health` - Health check

### Orchestration Engine (Port 8080)

- `POST /graphql` - GraphQL endpoint for data requests
- `GET /health` - Health check

## Quick API Examples

```bash
# Policy Decision
curl -X POST http://localhost:8082/decide \
  -H "Content-Type: application/json" \
  -d '{
    "consumer_id": "passport-app",
    "app_id": "passport-app",
    "request_id": "req_123",
    "required_fields": ["person.fullName", "person.photo"]
  }'

# Consent Management
curl -X POST http://localhost:8081/consents \
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

## Development

### Local Development

```bash
# Start services
make start

# Run tests
make test

# Stop services
make stop

# View logs
make logs
```

### Environment Configuration

```bash
# Local development
docker compose --env-file .env.local up --build

# Production testing
docker compose --env-file .env.production up --build
```

### Testing

#### Unit Tests

```bash
# Policy Decision Point
cd policy-decision-point && go test -v

# Consent Engine
cd consent-engine && go test -v
```

#### Integration Tests

```bash
# All integration tests
cd ../tests/integration && docker compose -f docker-compose.test.yml up -d && go test -v ./... && docker compose -f docker-compose.test.yml down -v
```

## Production Deployment

### Choreo Deployment

```bash
# Prepare for Choreo
./scripts/prepare-docker-build.sh

# Commit and push
git add . && git commit -m "Deploy to Choreo" && git push

# Restore local development
./scripts/restore-local-build.sh
```

### Docker Configuration

- **Build Context**: Repository root (`.`)
- **Dockerfile Path**: `exchange/{service}/Dockerfile`
- **Shared Dependencies**: Copied from `shared/` directory

## Configuration

### Environment Variables

**Local (`.env.local`):**

```bash
ENVIRONMENT=local
LOG_LEVEL=debug
LOG_FORMAT=text
```

**Production (`.env.production`):**

```bash
ENVIRONMENT=production
LOG_LEVEL=warn
LOG_FORMAT=json
```

## Scripts

| Script                    | Purpose                                     |
| ------------------------- | ------------------------------------------- |
| `manage.sh`               | Service management (start/stop/status/logs) |
| `test.sh`                 | API testing                                 |
| `restore-local-build.sh`  | Restore to local development state          |
| `prepare-docker-build.sh` | Prepare for production deployment           |

## Troubleshooting

**Issue: "shared directory not found" in Choreo build**

- **Solution**: Ensure build context is set to repository root (`.`) in Choreo console

**Issue: Local tests fail with import errors**

- **Solution**: Run `./scripts/restore-local-build.sh` to restore local development setup

**Issue: Docker build fails with go.mod errors**

- **Solution**: Run `./scripts/prepare-docker-build.sh` before building for Choreo

## Directory Structure

```
exchange/
├── policy-decision-point/    # Policy service (Port 8082)
├── consent-engine/           # Consent service (Port 8081)
├── orchestration-engine/  # Orchestration service (Port 8080)
├── shared/                   # Shared utilities and packages
├── scripts/                  # Management scripts
├── docker-compose.yml        # Multi-environment orchestration
└── Makefile                  # Convenience commands
```

## Related Documentation

- [Policy Decision Point README](policy-decision-point/README.md)
- [Consent Engine README](consent-engine/README.md)
- [Orchestration Engine README](orchestration-engine/README.md)
- [Integration Tests README](../tests/integration/README.md)
- [Scripts README](scripts/README.md)
