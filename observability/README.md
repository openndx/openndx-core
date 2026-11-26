# Observability

A local Prometheus and Grafana for the services' metrics. Only the Consent Engine serves metrics today.

## Quick start

Start the platform first, with `make up` or `docker compose up -d`: this stack joins the platform's `openndx-network`. Then:

```bash
cd observability
docker compose up -d
```

| Service | URL | Notes |
|---|---|---|
| Prometheus | http://localhost:9095 | Scrapes the Consent Engine's `/metrics` every 15 seconds |
| Grafana | http://localhost:3002 | Sign in with `GRAFANA_ADMIN_USER` and `GRAFANA_ADMIN_PASSWORD`, `admin` and `admin` by default. The Prometheus data source and the dashboards in `grafana/dashboards/` are provisioned. |

`docker compose down` stops the stack; add `-v` to delete the stored metrics and Grafana's data.

## Metrics

The Consent Engine serves `/metrics` in the Prometheus format, through the shared [`internal/monitoring`](../internal/monitoring) package, which uses the OpenTelemetry SDK. It records every request:

| Metric | Type | Labels |
|---|---|---|
| `http_requests_ratio_total` | Counter | `http_request_method`, `http_route`, `http_response_status_code` |
| `http_request_duration_seconds` | Histogram | `http_request_method`, `http_route` |

`http_route` is the route template, such as `/api/v1/consents/:id`, so IDs don't create new series. Every metric also has an `otel_scope_name` label, and the `target_info` metric gives the service's `service_name` and `service_version`.

The package also defines `external_calls_total`, `external_call_errors_total`, `external_call_duration_seconds` and `business_events_total`, which `monitoring.RecordExternalCall` and `monitoring.RecordBusinessEvent` record. No service calls them yet.

## Settings

A service that uses the package reads these environment variables the first time it records or serves metrics:

| Variable | Default | Meaning |
|---|---|---|
| `OTEL_METRICS_EXPORTER` | `prometheus` | `prometheus` serves the metrics at `/metrics`, `otlp` pushes them to an OTLP endpoint, and `none` turns metrics off. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | | The OTLP endpoint. Required for `otlp`, and it must use `https` unless `OTEL_EXPORTER_OTLP_INSECURE` is `true`. |
| `OTEL_EXPORTER_OTLP_HEADERS` | | Extra headers for `otlp`, such as an API key, as `key1=value1,key2=value2`. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `false` | `true` allows an `http` OTLP endpoint. For development only. |
| `SERVICE_NAME` | `openndx-service` | The `service_name` on `target_info`. |
| `SERVICE_VERSION` | `dev` | The `service_version` on `target_info`. |

Compose sets none of them for the Consent Engine, so its metrics report `service_name="openndx-service"`.

## Adding metrics to a service

1. In the service's `main.go`, serve the metrics and wrap the router, as [`cmd/ce/main.go`](../cmd/ce/main.go) does:

   ```go
   mux.Handle("/metrics", monitoring.Handler())
   handler := monitoring.HTTPMetricsMiddleware(mux)
   ```

2. Record calls to other systems with `monitoring.RecordExternalCall(target, operation, duration, err)`, and business events with `monitoring.RecordBusinessEvent(action, outcome)`.
3. Add a scrape job for the service to [`prometheus/prometheus.yml`](prometheus/prometheus.yml), using its Compose service name and port, and run `docker compose restart prometheus`.

`/metrics` isn't authenticated, so keep it off any public network.
