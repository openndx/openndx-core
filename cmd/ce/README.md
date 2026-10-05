# Consent Engine

Manages data owner consent workflows for data access requests. Provides JWT-authenticated endpoints for user interactions and internal APIs for service-to-service communication.

## Quick Start

```bash
# Run locally (from the repo root)
go run ./cmd/ce

# Or build and run
go build -o ce ./cmd/ce && ./ce
```

Service runs on port **8081** by default.

## Configuration

### Environment Variables

| Variable             | Description                                                             | Default                 |
|----------------------|-------------------------------------------------------------------------|-------------------------|
| `PORT`               | Service port                                                            | `8081`                  |
| `ENVIRONMENT`        | `production` or `local`                                                 | `local`                 |
| `CONSENT_PORTAL_URL` | Consent Portal URL                                                      | `http://localhost:5173` |
| `IDP_ORG_NAME`       | IDP organization name                                                   | -                       |
| `IDP_ISSUER`         | JWT issuer URL                                                          | -                       |
| `IDP_AUDIENCE`       | JWT audience                                                            | -                       |
| `IDP_JWKS_URL`       | JWKS endpoint URL                                                       | -                       |
| `IDP_SUBJECT_CLAIM`  | Token claim carrying the owner UID (matched against consent `owner_id`) | `sub`                   |
| `DB_HOST`            | Database host                                                           | `localhost`             |
| `DB_PORT`            | Database port                                                           | `5432`                  |
| `DB_USERNAME`        | Database username                                                       | `postgres`              |
| `DB_PASSWORD`        | Database password                                                       | -                       |
| `DB_NAME`            | Database name                                                           | `consent_engine`        |
| `DB_SSLMODE`         | SSL mode                                                                | `require`               |

## API Endpoints

### Internal APIs (No Authentication)

| Method | Endpoint                    | Description               |
|--------|-----------------------------|---------------------------|
| GET    | `/internal/api/v1/health`   | Health check              |
| GET    | `/internal/api/v1/consents` | Get consent by session ID |
| POST   | `/internal/api/v1/consents` | Create new consent        |

### Portal APIs (JWT Authentication)

| Method | Endpoint                       | Description                                                                     |
|--------|--------------------------------|---------------------------------------------------------------------------------|
| GET    | `/api/v1/health`               | Health check                                                                    |
| GET    | `/api/v1/consents`             | List the signed-in owner's consents (`?status=pending&limit=20&offset=0`)       |
| GET    | `/api/v1/consents/{consentId}` | Get consent details                                                             |
| PUT    | `/api/v1/consents/{consentId}` | Update consent status (pending only; `409 CONSENT_NOT_PENDING` otherwise)       |

### System Endpoints

| Method | Endpoint   | Description         |
|--------|------------|---------------------|
| GET    | `/health`  | Legacy health check |
| GET    | `/metrics` | Prometheus metrics  |

## Testing

```bash
# From the repo root
go test ./cmd/ce/... ./internal/ce/... -count=1
```

## Docker

```bash
# Build from the repo root
docker build -t consent-engine -f cmd/ce/Dockerfile .

# Run
docker run -p 8081:8081 --env-file .env consent-engine
```