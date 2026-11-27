# audit

The audit records of the Orchestration Engine (OE) and the Portal Backend (PB): what each service records, what a record contains, and how records are exported. The package writes each record as an [`slog`](https://pkg.go.dev/log/slog) record marked `category=audit`. The services send these records only to an OpenTelemetry endpoint, never to their own logs.

## What is recorded

### Orchestration Engine

Every consumer query produces these records, and the records of one query share a `traceID`:

| Event | Written | Actor (`actorType`, `actorID`) | Target (`targetType`, `targetID`) | Metadata |
|---|---|---|---|---|
| `DATA_REQUEST` | When the query arrives, before it is parsed | `APPLICATION`, the token's `client_id` | `SERVICE`, `SERVICE` | The application ID |
| `POLICY_CHECK` | After the Policy Decision Point's decision | `audit.actorType`, `audit.actorId` (default `SERVICE`, `orchestration-engine`) | `SERVICE`, `policy-decision-point` | The application ID and the fields asked for; the decision, with each field's outcome and obligations, such as `audit.consent_bypassed` |
| `CONSENT_CHECK` | After the Consent Engine call, when consent is required | The same | `SERVICE`, `consent-engine` | The application ID, the owner ID (the value of the query argument named by `consentEngine.ownerIdArgName`) and the number of fields; the consent's ID, status and portal URL |
| `PROVIDER_FETCH` | After each provider sub-query | The same | `SERVICE`, the provider's schema ID | The application ID, schema ID and fields asked for; whether the provider returned errors, how many, the first three as the provider returned them, and the top-level keys of its `data` |

A record's `status` is `FAILURE` when the call fails, in which case the metadata holds the error message. It is also `FAILURE` when the application isn't allowed a field or its access has expired (`POLICY_CHECK`), and when the provider returns GraphQL errors (`PROVIDER_FETCH`). Otherwise it is `SUCCESS`.

No record holds the query text or the values in a provider's `data`.

### Portal Backend

PB writes one `MANAGEMENT_EVENT` record for each `POST`, `PUT`, `PATCH` or `DELETE` request that reaches its service call, whether the call succeeds or fails:

- **`action`:** `CREATE` for `POST`, `UPDATE` for `PUT` and `PATCH`, and `DELETE` for `DELETE`. Revoking a consent bypass grant is a `POST`, so it is recorded as `CREATE`.
- **Actor:** the token's `sub`, with the actor type described in [AUTHORIZATION.md](../../cmd/pb/AUTHORIZATION.md#audit).
- **Target:** `RESOURCE`, and the resource's ID when it is known.
- **Metadata:** `resource` (`MEMBERS`, `SCHEMAS`, `SCHEMA-SUBMISSIONS`, `APPLICATIONS`, `APPLICATION-SUBMISSIONS` or `CONSENT-BYPASS-GRANTS`) and `clientId`, the token's `client_id`.

A request rejected before the service call leaves no record, for example one with an invalid body, or one that fails authentication or a scope or ownership check. PB records have no `traceID`. `GET` requests are never recorded.

## Record format

Each record has the message `audit` and these attributes:

| Attribute | Present | Value |
|---|---|---|
| `category` | Always | `audit` |
| `eventType` | Always | `DATA_REQUEST`, `POLICY_CHECK`, `CONSENT_CHECK`, `PROVIDER_FETCH` or `MANAGEMENT_EVENT` |
| `action` | PB only | `CREATE`, `UPDATE` or `DELETE` |
| `status` | Always | `SUCCESS` or `FAILURE` |
| `actorType`, `actorID` | Always | See above |
| `targetType` | Always | `SERVICE` (OE) or `RESOURCE` (PB) |
| `targetID` | When known | See above |
| `traceID` | OE only | Shared by the records of one query |
| `metadata` | When set | See above |

The attribute names are those of the `LogSink` in [`OpenNSW/core/platform/audit`](https://github.com/OpenNSW/core/tree/main/platform/audit), so a query on `category='audit'` finds the audit records of OpenNDX and the NSW alike. The NSW's records differ in two ways: they always carry `action`, and they can carry a `message`.

## Export

The services pass the package the OTLP/HTTP log exporter of [`OpenNSW/core/telemetry`](https://github.com/OpenNSW/core/tree/main/telemetry), configured by the `otel` section of their config. The keys are in [cmd/oe/CONFIGURATION.md](../../cmd/oe/CONFIGURATION.md#otel) and [cmd/pb/CONFIGURATION.md](../../cmd/pb/CONFIGURATION.md#opentelemetry-and-audit).

- **Export off.** When `otel` sets no endpoint for logs, export is off. The service logs `otel log export is off: audit records (category=audit) are not retained` at startup and drops every record.
- **Batching.** Records wait in a queue of up to 2048 and are sent in batches every second.
  - A failed export is retried for up to a minute.
  - When the queue is full, the oldest records are dropped.
  - Records still queued are lost if the process stops without flushing them.
- **Shutdown.** The OE flushes the queue when it shuts down, for up to 5 seconds. PB flushes it last in its shutdown, but exits without flushing when its HTTP server fails to shut down.
- **Environment variables.** `otel` is the only source for the endpoint, headers and service name, so the SDK ignores `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS` and `OTEL_SERVICE_NAME`. To take one of them from the environment, reference it in the config file, for example `endpoint: "{{env:OTEL_EXPORTER_OTLP_ENDPOINT}}"`. The variables `otel` has no key for still apply, such as:
  - `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_EXPORTER_OTLP_TIMEOUT` and `OTEL_EXPORTER_OTLP_COMPRESSION`;
  - the TLS certificate variables;
  - the batch settings `OTEL_BLRP_MAX_QUEUE_SIZE`, `OTEL_BLRP_SCHEDULE_DELAY`, `OTEL_BLRP_EXPORT_TIMEOUT` and `OTEL_BLRP_MAX_EXPORT_BATCH_SIZE`.
- **Metrics export.** The OE and PB record no traces or metrics. While `otel.endpoint` is set, though, the SDK posts an empty metrics export to `/v1/metrics` every 60 seconds.

To see the records locally, follow [Viewing audit records](../../CONTRIBUTING.md#viewing-audit-records).

## Using the package

- `audit.NewExportSink(tel.SlogHandler())` returns the sink a service writes to. A `nil` handler means log export is off: it logs the warning above and returns a sink that discards every record.
- `Sink.Write(ctx, audit.Record{...})` writes one record. `Action`, `TargetID`, `TraceID` and `Metadata` are left out when empty, and the record is written even if `ctx` is cancelled.
- `CaptureSink` keeps the records written to it, for tests.
- Keep the attribute names in step with `OpenNSW/core/platform/audit`, and keep query text and provider data out of the metadata.
