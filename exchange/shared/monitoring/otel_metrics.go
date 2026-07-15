package monitoring

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// Custom OpenTelemetry attributes for OpenDIF-specific metrics.
// These attributes use the "opendif." namespace prefix to distinguish them
// from standard OpenTelemetry semantic conventions.
//
// Custom Attributes:
//   - opendif.business.action: The business action being performed (e.g., "consent_created", "policy_decision")
//   - opendif.business.outcome: The outcome of the business action (e.g., "success", "failure", "allow", "deny")
//   - opendif.external.target: The target system/service for external calls (e.g., "postgres", "redis", "external-api")
//   - opendif.external.operation: The operation type for external calls (e.g., "query", "insert", "get", "set")
//
// Standard semantic conventions (from semconv package) are used for HTTP metrics:
//   - http.method, http.route, http.status_code (via semconv.HTTPRequestMethodKey, etc.)
const (
	// Attribute keys for custom OpenDIF metrics
	attrBusinessAction    = "opendif.business.action"
	attrBusinessOutcome   = "opendif.business.outcome"
	attrExternalTarget    = "opendif.external.target"
	attrExternalOperation = "opendif.external.operation"
)

var (
	// Metrics instruments
	httpRequestsCounter   metric.Int64Counter
	httpRequestDuration   metric.Float64Histogram
	externalCallsCounter  metric.Int64Counter
	externalCallErrors    metric.Int64Counter
	externalCallDuration  metric.Float64Histogram
	businessEventsCounter metric.Int64Counter
	metricsHandler        http.Handler
	initialized           int32     // Use atomic int32 for thread-safe reads/writes
	otelInitOnce          sync.Once // Separate sync.Once for OpenTelemetry initialization
)

// Config holds the configuration for OpenTelemetry metrics
type Config struct {
	// ExporterType can be "prometheus", "otlp", or "none" (disabled)
	ExporterType string
	// ServiceName is the name of the service (e.g., "portal-backend", "orchestration-engine")
	ServiceName string
	// ServiceVersion is the version of the service (e.g., "1.0.0", "v2.3.1")
	// Defaults to "dev" if not set via SERVICE_VERSION environment variable
	ServiceVersion string
	// OTLPEndpoint is the OTLP endpoint URL (for Datadog, New Relic, etc.)
	// Example: "https://api.datadoghq.com/api/v2/otlp"
	OTLPEndpoint string
	// OTLPHeaders are additional headers for OTLP exporter (e.g., API keys)
	OTLPHeaders map[string]string
	// PrometheusPort is the port for Prometheus exporter (default: 8888)
	PrometheusPort int
	// OTLPTLSInsecure allows insecure TLS connections (only for development/testing)
	// Set via OTEL_EXPORTER_OTLP_INSECURE environment variable
	OTLPTLSInsecure bool
	// HistogramBuckets allows customization of histogram bucket boundaries (in seconds)
	// Default: [.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10]
	// These boundaries are optimized for HTTP request latency measurements:
	// - Sub-10ms: .005, .01, .025 (very fast responses)
	// - 10-100ms: .05, .1, .25 (typical API responses)
	// - 100ms-1s: .5, 1 (slower operations)
	// - 1s+: 2.5, 5, 10 (long-running operations, timeouts)
	HistogramBuckets []float64
}

// DefaultConfig returns a default configuration
func DefaultConfig(serviceName string) Config {
	return Config{
		ExporterType:     getEnvOrDefault("OTEL_METRICS_EXPORTER", "prometheus"),
		ServiceName:      serviceName,
		ServiceVersion:   getEnvOrDefault("SERVICE_VERSION", "dev"),
		OTLPEndpoint:     getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		PrometheusPort:   8888,
		OTLPHeaders:      parseHeaders(getEnvOrDefault("OTEL_EXPORTER_OTLP_HEADERS", "")),
		OTLPTLSInsecure:  getEnvBoolOrDefault("OTEL_EXPORTER_OTLP_INSECURE", false),
		HistogramBuckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}
}

// Initialize sets up OpenTelemetry metrics with the given configuration
// This function is thread-safe and can be called multiple times safely.
// Only the first call will perform initialization; subsequent calls return nil.
func Initialize(config Config) error {
	// Use sync.Once to ensure initialization only happens once
	var initErr error
	otelInitOnce.Do(func() {
		ctx := context.Background()
		initErr = initializeInternal(ctx, config)
		if initErr == nil {
			atomic.StoreInt32(&initialized, 1)
		}
	})

	return initErr
}

// initializeInternal performs the actual initialization work
func initializeInternal(ctx context.Context, config Config) error {
	// Create resource with service name and version
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create meter provider based on exporter type
	var reader sdkmetric.Reader
	var handler http.Handler

	switch config.ExporterType {
	case "prometheus", "":
		// Use Prometheus exporter (default for local dev)
		// Create a Prometheus registry for the exporter
		reg := prometheus.NewRegistry()
		exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
		if err != nil {
			return fmt.Errorf("failed to create Prometheus exporter: %w", err)
		}
		reader = exporter
		// Use promhttp.HandlerFor with the custom registry
		handler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
		metricsHandler = handler
		slog.Info("Initialized OpenTelemetry metrics with Prometheus exporter",
			"service", config.ServiceName)

	case "otlp":
		// Use OTLP exporter (for Datadog, New Relic, etc.)
		if config.OTLPEndpoint == "" {
			return fmt.Errorf("OTLP endpoint is required when using OTLP exporter")
		}

		// Parse endpoint URL
		endpointURL, err := url.Parse(config.OTLPEndpoint)
		if err != nil {
			return fmt.Errorf("invalid OTLP endpoint URL: %w", err)
		}

		// Security: Require HTTPS by default for all endpoints
		// Only allow insecure connections if explicitly enabled via OTEL_EXPORTER_OTLP_INSECURE
		if endpointURL.Scheme != "https" {
			if !config.OTLPTLSInsecure {
				return fmt.Errorf("OTLP endpoint must use HTTPS (got: %s). Use https:// for secure connections, or set OTEL_EXPORTER_OTLP_INSECURE=true to allow insecure connections (not recommended for production)", endpointURL.Scheme)
			}
			// Insecure connection explicitly enabled via environment variable
			slog.Warn("Using insecure HTTP connection for OTLP endpoint (OTEL_EXPORTER_OTLP_INSECURE=true)",
				"endpoint", config.OTLPEndpoint,
				"warning", "This disables TLS verification and exposes metrics data in transit")
		}

		// Extract host:port from URL (WithEndpoint expects host:port, not full URL)
		// The scheme is controlled by WithInsecure() option
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpoint(endpointURL.Host),
		}

		// Only use WithInsecure() if explicitly enabled via environment variable
		if config.OTLPTLSInsecure && endpointURL.Scheme == "http" {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		// For HTTPS endpoints (default), TLS with proper certificate validation is used automatically

		// Add headers if provided
		if len(config.OTLPHeaders) > 0 {
			opts = append(opts, otlpmetrichttp.WithHeaders(config.OTLPHeaders))
		}

		exporter, err := otlpmetrichttp.New(ctx, opts...)
		if err != nil {
			return fmt.Errorf("failed to create OTLP exporter: %w", err)
		}

		reader = sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(15*time.Second))
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("# Metrics exported via OTLP\n"))
		})
		slog.Info("Initialized OpenTelemetry metrics with OTLP exporter",
			"service", config.ServiceName,
			"endpoint", config.OTLPEndpoint,
			"insecure", config.OTLPTLSInsecure)

	case "none":
		// Disabled - use no-op reader
		reader = sdkmetric.NewManualReader()
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("# Metrics disabled\n"))
		})
		slog.Info("OpenTelemetry metrics disabled",
			"service", config.ServiceName)

	default:
		return fmt.Errorf("unknown exporter type: %s (supported: prometheus, otlp, none)", config.ExporterType)
	}

	// Use default histogram buckets if not configured
	histogramBuckets := config.HistogramBuckets
	if len(histogramBuckets) == 0 {
		histogramBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	}

	// Create meter provider with custom histogram buckets for all duration metrics
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Name: "http_request_duration_seconds"},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
					Boundaries: histogramBuckets,
				},
			},
		)),
		sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Name: "external_call_duration_seconds"},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
					Boundaries: histogramBuckets,
				},
			},
		)),
	)

	// Set global meter provider
	otel.SetMeterProvider(meterProvider)

	// Create meter
	meter := otel.Meter("opendif")

	// Create instruments
	httpRequestsCounter, err = meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create http_requests_total counter: %w", err)
	}

	httpRequestDuration, err = meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("failed to create http_request_duration_seconds histogram: %w", err)
	}

	externalCallsCounter, err = meter.Int64Counter(
		"external_calls_total",
		metric.WithDescription("Total number of external service calls"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create external_calls_total counter: %w", err)
	}

	externalCallErrors, err = meter.Int64Counter(
		"external_call_errors_total",
		metric.WithDescription("Total number of failed external service calls"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create external_call_errors_total counter: %w", err)
	}

	externalCallDuration, err = meter.Float64Histogram(
		"external_call_duration_seconds",
		metric.WithDescription("External service call duration in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("failed to create external_call_duration_seconds histogram: %w", err)
	}

	businessEventsCounter, err = meter.Int64Counter(
		"business_events_total",
		metric.WithDescription("Total number of business events"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return fmt.Errorf("failed to create business_events_total counter: %w", err)
	}

	return nil
}

// otelHandler returns the metrics HTTP handler
// For Prometheus exporter, this returns the Prometheus metrics endpoint
// For OTLP exporter, this returns a simple status endpoint
func otelHandler() http.Handler {
	if atomic.LoadInt32(&initialized) == 0 || metricsHandler == nil {
		// Fallback if not initialized
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("# Metrics not initialized\n"))
		})
	}
	return metricsHandler
}

// otelHTTPMetricsMiddleware wraps an HTTP handler to record metrics using OpenTelemetry
func otelHTTPMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&initialized) == 0 {
			// If metrics not initialized, just pass through
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		// Wrap ResponseWriter to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call the next handler
		next.ServeHTTP(rw, r)

		// Record metrics
		duration := time.Since(start).Seconds()
		method := r.Method

		// Normalize route, but use "unknown" for 404s to prevent cardinality explosion
		route := normalizeRoute(r.URL.Path)
		if rw.statusCode == http.StatusNotFound {
			route = "unknown"
		}

		// Record metrics with attributes
		httpRequestsCounter.Add(context.Background(), 1,
			metric.WithAttributes(
				semconv.HTTPRequestMethodKey.String(method),
				semconv.HTTPRouteKey.String(route),
				semconv.HTTPResponseStatusCodeKey.Int(rw.statusCode),
			),
		)
		httpRequestDuration.Record(context.Background(), duration,
			metric.WithAttributes(
				semconv.HTTPRequestMethodKey.String(method),
				semconv.HTTPRouteKey.String(route),
			),
		)
	})
}

// otelRecordExternalCall records an external service call using OpenTelemetry
func otelRecordExternalCall(target, operation string, duration time.Duration, err error) {
	if atomic.LoadInt32(&initialized) == 0 {
		return
	}

	ctx := context.Background()
	attrs := metric.WithAttributes(
		attribute.String(attrExternalTarget, target),
		attribute.String(attrExternalOperation, operation),
	)

	externalCallsCounter.Add(ctx, 1, attrs)
	externalCallDuration.Record(ctx, duration.Seconds(), attrs)
	if err != nil {
		externalCallErrors.Add(ctx, 1, attrs)
	}
}

// otelRecordBusinessEvent records a business event using OpenTelemetry
func otelRecordBusinessEvent(action, outcome string) {
	if atomic.LoadInt32(&initialized) == 0 {
		return
	}

	businessEventsCounter.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String(attrBusinessAction, action),
			attribute.String(attrBusinessOutcome, outcome),
		),
	)
}

// Helper functions
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	if headerStr == "" {
		return headers
	}

	// Parse format: "key1=value1,key2=value2"
	pairs := strings.Split(headerStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return headers
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	// Accept common boolean representations
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "true" || value == "1" || value == "yes" || value == "on"
}
