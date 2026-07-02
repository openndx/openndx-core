# Observability Stack for OpenDIF Core

Local development stack: **Go Services** → **OpenTelemetry** → **Prometheus** → **Grafana**

Collects real-time metrics from all Go services for debugging performance and errors. Uses **OpenTelemetry** for vendor-agnostic metrics collection, allowing you to switch between Prometheus (default), Datadog, New Relic, or any OTLP-compatible backend without changing code.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Go Services (HTTP Servers)                                                 │
│  ┌──────────────┐  ┌───────────────┐  ┌──────────────┐                   │
│  │ Portal       │  │ Orchestration │  │ Policy        │  ...               │
│  │ Backend      │  │ Engine        │  │ Decision      │                    │
│  │ :3000        │  │ :4000         │  │ Point :8082   │                    │
│  └──────┬───────┘  └───────┬───────┘  └──────┬───────┘                    │
│         │                  │                 │                             │
│         └──────────────────┴─────────────────┘                            │
│                    │                                                        │
│                    │ HTTP Requests                                         │
│                    │ (with OpenTelemetry Middleware)                       │
│                    ▼                                                        │
│         ┌──────────────────────────────┐                                   │
│         │  OpenTelemetry SDK            │                                   │
│         │  (Vendor-Agnostic)            │                                   │
│         └──────────────┬────────────────┘                                   │
│                        │                                                    │
│                        │ Exporter (Configurable)                            │
│                        ▼                                                    │
│         ┌─────────────────────────────────────────────┐                    │
│         │  /metrics endpoint                           │                    │
│         │  (Format depends on exporter)               │                    │
│         └─────────────────────────────────────────────┘                    │
└────────────────────────┬────────────────────────────────────────────────────┘
                         │
                         │ Export (scrape or push)
                         │
         ┌───────────────┼───────────────┬───────────────┐
         │               │               │               │
         ▼               ▼               ▼               ▼
┌────────────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
│  Prometheus    │ │ Datadog  │ │New Relic │ │  Other   │
│  (Default)     │ │ (OTLP)   │ │ (OTLP)   │ │ (OTLP)   │
│  :9091         │ │          │ │          │ │          │
│                │ │          │ │          │ │          │
│  Scrapes       │ │  Pushes  │ │  Pushes  │ │  Pushes  │
│  /metrics      │ │  via     │ │  via     │ │  via     │
│  every 15s     │ │  OTLP    │ │  OTLP    │ │  OTLP    │
└────────┬───────┘ └──────────┘ └──────────┘ └──────────┘
         │
         │ PromQL Queries
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  Grafana (localhost:3002)                                                  │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Data Source: Prometheus (for local dev)                             │  │
│  │  URL: http://prometheus:9090                                         │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                            │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Dashboards                                                          │  │
│  │  - Go Services Metrics                                               │  │
│  │  - HTTP Traffic, Latency, Errors                                    │  │
│  │  - Service Health                                                    │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Key Points:**
- **OpenTelemetry SDK** provides vendor-agnostic instrumentation
- **Exporter** determines where metrics go (Prometheus, Datadog, New Relic, etc.)
- **No code changes** needed to switch backends - just environment variables
- **Default**: Prometheus exporter for local development
- **Production**: Switch to OTLP exporter for Datadog/New Relic/etc.

---

## Quick Start

```bash
cd observability
docker compose up -d
```

**Services:**
- **Prometheus**: http://localhost:9091 (raw metrics & queries)
- **Grafana**: http://localhost:3002 (dashboards, login: `admin` / `admin`)

**Prerequisites:**

Ensure all Go services are running and connected to the `opendif-network`:
- Orchestration Engine (port 4000)
- Consent Engine (port 8081)
- Policy Decision Point (port 8082)
- Portal Backend (port 3000)

---

## Switching Observability Backends

The observability stack uses **OpenTelemetry**, allowing you to switch between different backends without changing code. Configure via environment variables.

### Default: Prometheus (Local Development)

No configuration needed! Services automatically use Prometheus exporter by default.

```bash
# Services expose metrics at /metrics endpoint
# Prometheus scrapes every 15 seconds
# View in Grafana: http://localhost:3002
```

### Switch to Datadog (Production)

Set environment variables before starting services:

```bash
export OTEL_METRICS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=https://api.datadoghq.com/api/v2/otlp
export OTEL_EXPORTER_OTLP_HEADERS="DD-API-KEY=your-api-key,DD-SITE=datadoghq.com"
export SERVICE_NAME=portal-backend

# Start your service
./your-service
```

**Alternative (via Datadog Agent):**
```bash
export OTEL_METRICS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318  # Datadog Agent OTLP HTTP endpoint
export SERVICE_NAME=portal-backend
```

### Switch to New Relic

```bash
export OTEL_METRICS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.nr-data.net
export OTEL_EXPORTER_OTLP_HEADERS="api-key=your-newrelic-license-key"
export SERVICE_NAME=portal-backend

# Start your service
./your-service
```

### Disable Metrics

```bash
export OTEL_METRICS_EXPORTER=none
```

### Configuration Reference

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `OTEL_METRICS_EXPORTER` | Exporter type: `prometheus`, `otlp`, or `none` | `prometheus` | `otlp` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint URL (required for `otlp` exporter) | - | `https://api.datadoghq.com/api/v2/otlp` |
| `OTEL_EXPORTER_OTLP_HEADERS` | OTLP headers (e.g., API keys). Format: `key1=value1,key2=value2` | - | `DD-API-KEY=xxx,DD-SITE=datadoghq.com` |
| `SERVICE_NAME` | Service name for metrics | `opendif-service` | `portal-backend` |

**Note:** For Docker Compose deployments, add these environment variables to your `docker-compose.yml`:

```yaml
services:
  your-service:
    environment:
      - OTEL_METRICS_EXPORTER=otlp
      - OTEL_EXPORTER_OTLP_ENDPOINT=https://api.datadoghq.com/api/v2/otlp
      - OTEL_EXPORTER_OTLP_HEADERS=DD-API-KEY=xxx,DD-SITE=datadoghq.com
      - SERVICE_NAME=your-service
```

---

## Metrics Overview

### HTTP Request Metrics

| Metric                          | Type      | Labels                                    | Purpose                    |
| ------------------------------- | --------- | ----------------------------------------- | -------------------------- |
| `http_requests_total`           | Counter   | `http_method`, `http_route`, `http_status_code` | Request volume by endpoint |
| `http_request_duration_seconds` | Histogram | `http_method`, `http_route`               | API latency percentiles    |

**Label Definitions:**
- `http_method`: HTTP method (GET, POST, PUT, DELETE, etc.)
- `http_route`: Normalized route path (e.g., `/api/v1/members`, `/api/v1/policies`)
- `http_status_code`: HTTP response status code (200, 404, 500, etc.)

### External Call Metrics (Exchange services)

| Metric                            | Type      | Labels                                    | Purpose                    |
| --------------------------------- | --------- | ----------------------------------------- | -------------------------- |
| `external_calls_total`            | Counter   | `external_target`, `external_operation` | External call volume       |
| `external_call_duration_seconds`   | Histogram | `external_target`, `external_operation`    | External call latency      |
| `external_call_errors_total`       | Counter   | `external_target`, `external_operation`    | Failed external calls      |

**Label Definitions:**
- `external_target`: Target service or system (e.g., `postgres`, `redis`, `external-api`)
- `external_operation`: Operation type (e.g., `query`, `insert`, `get`, `set`)

### Business Event Metrics (Exchange services)

| Metric                    | Type    | Labels                          | Purpose                |
| ------------------------- | ------- | ------------------------------- | ---------------------- |
| `business_events_total`   | Counter | `business_action`, `business_outcome` | Business KPI tracking |

**Label Definitions:**
- `business_action`: Business action type (e.g., `consent_created`, `policy_evaluated`)
- `business_outcome`: Outcome of the action (e.g., `success`, `failure`, `pending`)

### Go Runtime Metrics (Automatic)

The monitoring package automatically instruments Go runtime metrics:
- `process_cpu_seconds_total` - CPU usage
- `go_memstats_*` - Memory statistics (alloc, sys, heap, etc.)
- `go_goroutines` - Goroutine count
- `go_gc_duration_seconds` - Garbage collection pause times

---

## Useful Prometheus Queries

**Request Rate by Endpoint:**
```promql
sum by (http_route, http_method) (rate(http_requests_total[5m]))
```

**95th Percentile Latency by Endpoint:**
```promql
histogram_quantile(0.95, sum by (http_route, le) (rate(http_request_duration_seconds_bucket[5m])))
```

**Error Rate by Endpoint:**
```promql
sum by (http_route) (rate(http_requests_total{http_status_code=~"5.."}[5m]))
```

**Top 10 Slowest Endpoints:**
```promql
topk(10, histogram_quantile(0.95, sum by (http_route, le) (rate(http_request_duration_seconds_bucket[5m]))))
```

**External Call Error Rate:**
```promql
sum by (external_target, external_operation) (rate(external_call_errors_total[5m]))
```

**Service Availability:**
```promql
up{job=~"orchestration-engine|consent-engine|policy-decision-point|portal-backend"}
```

---

## Grafana Dashboard

Pre-configured dashboard: **Go Services Metrics**

**URL:** http://localhost:3002/d/go-services/go-services-metrics

**Panels:**
- HTTP Traffic (req/s)
- HTTP Latency (P95)
- Service Health (1=up, 0=down)
- External Calls per Second
- External Call Error %
- Business Events

---

## Generating Sample Traffic

To populate the Grafana dashboard with metrics, generate sample traffic:

```bash
# From the observability directory
./generate_sample_traffic.sh
```

This sends requests to various endpoints on `portal-backend` (default: `http://localhost:3000`).

### Configuration

```bash
# Change the base URL
PORTAL_BACKEND_URL=http://localhost:3000 ./generate_sample_traffic.sh

# Change request interval (default: 2 seconds)
REQUEST_INTERVAL=5 ./generate_sample_traffic.sh

# Set number of request batches (default: 50, 0 = infinite)
REQUEST_COUNT=100 ./generate_sample_traffic.sh
```

### What the Script Does

The script sends requests to:
- **Health endpoints**: `/health`, `/metrics` (should return 200)
- **API endpoints**: `/api/v1/members`, `/api/v1/schemas`, etc. (may return 401 without auth, but still generates metrics)
- **Invalid endpoints**: `/api/v1/unknown` (generates 404s)

**Note**: Many API endpoints require authentication. The script will generate 401 Unauthorized responses, which is still useful for metrics (you'll see error rates, different status codes, etc.).

---

## Stop Services

```bash
docker compose down
```

**Keep data (volumes persist):**
```bash
docker compose stop
```

**Remove everything (data + volumes):**
```bash
docker compose down -v
```

---

## Production Deployment

This setup is for **local development only**. For production:

1. **Use Managed Service**: Grafana Cloud (free tier), Datadog, New Relic
2. **Or Self-Host**: Deploy Prometheus HA, Thanos/Mimir for long-term storage
3. **Security Hardening**: Change Grafana admin password, enable OAuth/SSO, use reverse proxy
4. **Storage & Retention**: Adjust `--storage.tsdb.retention.time` based on storage capacity
5. **Alerting**: Configure Alertmanager for production alerts

**Switching to Production Backend:**

Simply set the environment variables (see [Switching Observability Backends](#switching-observability-backends)) - no code changes needed!

---

## How It Works

### Service Instrumentation

All services use **OpenTelemetry** for metrics collection:

**Portal Backend** (`portal-backend/v1/middleware/otel_metrics.go`):
- OpenTelemetry metrics middleware wraps `/api/v1/` routes
- Records: `http_request_duration_seconds`, `http_requests_total`
- Attributes: `http.method`, `http.route`, `http.status_code`

**Exchange Services** (`exchange/shared/monitoring/otel_metrics.go`):
- Shared OpenTelemetry monitoring package with `HTTPMetricsMiddleware()`
- Records: `http_requests_total`, `http_request_duration_seconds`
- Additional metrics: `external_calls_total`, `business_events_total`
- Attributes: `http.method`, `http.route` (normalized), `http.status_code`

**Default Exporter:** Prometheus (for local dev). Configure via `OTEL_METRICS_EXPORTER` env var.

### Metrics Endpoint Exposure

Each instrumented service exposes a `/metrics` endpoint:

```go
// Portal Backend
topLevelMux.Handle("/metrics", v1middleware.MetricsHandler())

// Exchange Services
mux.Handle("/metrics", monitoring.Handler())
```

The endpoint returns metrics in **Prometheus text format** (when using Prometheus exporter):
```
http_requests_total{http_method="GET",http_route="/api/v1/members",http_status_code="200"} 42
http_request_duration_seconds_bucket{http_method="GET",http_route="/api/v1/members",le="0.1"} 38
...
```

### Prometheus Scraping

Prometheus periodically scrapes each service's `/metrics` endpoint:

- **Interval**: Every 15 seconds (configurable in `prometheus.yml`)
- **Method**: HTTP GET request to `http://<service>:<port>/metrics`
- **Configuration**: Defined in `prometheus/prometheus.yml` as scrape jobs
- **Network**: Uses Docker network (`opendif-network`) for service discovery

Example scrape config:
```yaml
- job_name: orchestration-engine
  metrics_path: /metrics
  static_configs:
    - targets:
        - orchestration-engine:4000
```

### Metric Storage

Prometheus stores scraped metrics in its Time-Series Database (TSDB):
- **Format**: Time-series with labels (e.g., `http_requests_total{method="GET",route="/api/v1/members"}`)
- **Retention**: 30 days (configurable)
- **Query Language**: PromQL (Prometheus Query Language)

### Grafana Visualization

Grafana queries Prometheus via PromQL to create dashboards:

- **Data Source**: Configured to connect to `http://prometheus:9090`
- **Queries**: Written in PromQL (e.g., `rate(http_requests_total[5m])`)
- **Dashboards**: Pre-configured panels showing:
  - HTTP request rates
  - Latency percentiles (P95, P99)
  - Error rates
  - Service health status

---

## How to Add Metrics to New Go Services

Services automatically initialize OpenTelemetry metrics when first used. No explicit initialization needed.

1. **For Exchange Services** - Use shared monitoring package:
   ```go
   import "github.com/gov-dx-sandbox/exchange/shared/monitoring"
   
   mux.Handle("/metrics", monitoring.Handler())
   handler := monitoring.HTTPMetricsMiddleware(mux)
   ```

2. **For Portal Backend** - Use middleware package:
   ```go
   import v1middleware "github.com/gov-dx-sandbox/portal-backend/v1/middleware"
   
   topLevelMux.Handle("/metrics", v1middleware.MetricsHandler())
   topLevelMux.Handle("/api/v1/", v1middleware.MetricsMiddleware(handler))
   ```

3. **Add service to Prometheus configuration:**
   Edit `prometheus/prometheus.yml`:
   ```yaml
   - job_name: your-service
     metrics_path: /metrics
     static_configs:
       - targets:
           - your-service:PORT
         labels:
           service: 'your-service'
           port: 'PORT'
   ```

4. **Ensure service is on `opendif-network`:**
   In your service's `docker-compose.yml`:
   ```yaml
   services:
     your-service:
       networks:
         - opendif-network
   
   networks:
     opendif-network:
       name: opendif-network
       external: true
   ```

5. **Restart Prometheus:**
   ```bash
   docker compose restart prometheus
   ```

---

## Troubleshooting

### Metrics not appearing

1. Check that metrics are initialized:
   - Look for log messages: "Initialized OpenTelemetry metrics with..."
   - Check `/metrics` endpoint returns data: `curl http://localhost:3000/metrics`

2. For OTLP exporter:
   - Verify `OTEL_EXPORTER_OTLP_ENDPOINT` is set correctly
   - Check network connectivity to the endpoint
   - Verify API keys/headers are correct

3. Check environment variables:
   ```bash
   env | grep OTEL
   ```

### Prometheus Can't Scrape Services

**Issue**: Targets show as DOWN in Prometheus (http://localhost:9091/targets)

**Solutions:**

1. Verify services are running and exposing metrics:
   ```bash
   curl http://localhost:4000/metrics
   curl http://localhost:8081/metrics
   curl http://localhost:8082/metrics
   ```

2. Check Prometheus logs:
   ```bash
   docker compose logs prometheus
   ```

3. Verify network connectivity:
   - Ensure all services are on the same `opendif-network`
   - Check service names match Prometheus configuration

### Grafana Can't Connect to Prometheus

**Issue**: "Data source is not working" in Grafana

**Solutions:**

1. Verify Prometheus is running: `curl http://localhost:9091/-/healthy`
2. Check datasource URL in `grafana/provisioning/datasources/datasource.yml` (should be `http://prometheus:9090`)
3. Ensure both containers are on the same Docker network (`opendif-network`)

### Network Issues

**Issue**: Services can't communicate with each other

**Solutions:**

1. Verify network exists: `docker network ls | grep opendif-network`
2. Check service is on network: `docker network inspect opendif-network`
3. Recreate network if needed:
   ```bash
   docker compose down
   docker network rm opendif-network
   docker compose up -d
   ```

---

## Additional Resources

- [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/instrumentation/go/)
- [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/)
- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [Datadog OTLP Ingest](https://docs.datadoghq.com/opentelemetry/otlp_ingest_in_the_agent/)
- [New Relic OTLP](https://docs.newrelic.com/docs/more-integrations/open-source-telemetry-integrations/opentelemetry/opentelemetry-setup/)
