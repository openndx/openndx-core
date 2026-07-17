# Consent Engine

Manages data owner consent workflows for data access requests. Provides JWT-authenticated endpoints for user interactions and internal APIs for service-to-service communication.

## Quick Start

```bash
# Run locally
go run main.go

# Or build and run
go build -o consent-engine && ./consent-engine
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

| Method | Endpoint                       | Description           |
|--------|--------------------------------|-----------------------|
| GET    | `/api/v1/health`               | Health check          |
| GET    | `/api/v1/consents/{consentId}` | Get consent details   |
| PUT    | `/api/v1/consents/{consentId}` | Update consent status |

### System Endpoints

| Method | Endpoint   | Description         |
|--------|------------|---------------------|
| GET    | `/health`  | Legacy health check |
| GET    | `/metrics` | Prometheus metrics  |

## Testing

```bash
go test ./...
```

## Docker

```bash
# Build from monorepo root
docker build -t consent-engine -f exchange/consent-engine/Dockerfile .

# Run
docker run -p 8081:8081 --env-file .env consent-engine
```