package monitoring

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	initOnce sync.Once
	initErr  error
)

var (
	// routesMu protects routes and routeTemplates
	routesMu sync.RWMutex
	// routes is a set of static routes that should be preserved as-is
	routes = make(map[string]bool)
	// routeTemplates is a set of route templates (e.g., "/api/v1/schema/:id")
	// that should be matched against incoming paths
	routeTemplates = make([]string, 0)
)

// ensureInitialized ensures OpenTelemetry is initialized with default config
// This is called automatically when metrics functions are used
func ensureInitialized() {
	initOnce.Do(func() {
		// Try to get service name from environment or use default
		serviceName := os.Getenv("SERVICE_NAME")
		if serviceName == "" {
			serviceName = "opendif-service"
		}

		config := DefaultConfig(serviceName)
		initErr = Initialize(config)
		if initErr != nil {
			slog.Error("Failed to initialize OpenTelemetry metrics, metrics will be disabled",
				"error", initErr,
				"service", serviceName,
				"impact", "Service will continue running but metrics collection is disabled")
		}
	})
}

// GetInitError returns the initialization error if metrics failed to initialize.
// Returns nil if initialization succeeded or hasn't been attempted yet.
// Services can call this to check if metrics are working and take appropriate action
// (e.g., fail to start if metrics are critical, or log a health check status).
func GetInitError() error {
	ensureInitialized()
	return initErr
}

// IsInitialized returns true if metrics have been successfully initialized.
// Returns false if initialization failed or hasn't been attempted yet.
func IsInitialized() bool {
	ensureInitialized()
	return initErr == nil
}

// RegisterRoutes registers routes for normalization. Supports static routes and templates with :id or {id} placeholders.
// Templates match incoming paths and normalize dynamic segments. Call during service initialization.
//
// Example: RegisterRoutes([]string{"/health", "/api/v1/schema/:id", "/api/v1/applications/{id}/activate"})
func RegisterRoutes(routesList []string) {
	routesMu.Lock()
	defer routesMu.Unlock()

	for _, route := range routesList {
		// Normalize {id} to :id for internal processing
		normalizedRoute := strings.ReplaceAll(route, "{id}", ":id")

		if strings.Contains(normalizedRoute, ":id") {
			// This is a template - store with normalized :id syntax
			routeTemplates = append(routeTemplates, normalizedRoute)
		} else {
			// This is a static route - stored for exact O(1) lookup
			routes[route] = true
		}
	}
}

// IsExactRoute checks if a route is exactly registered as a static route (no template matching).
func IsExactRoute(route string) bool {
	routesMu.RLock()
	defer routesMu.RUnlock()
	return routes[route]
}

// Handler returns the metrics HTTP handler
// This now uses OpenTelemetry under the hood, but maintains backward compatibility
// For Prometheus exporter, this returns the Prometheus metrics endpoint
// For OTLP exporter, this returns a simple status endpoint
func Handler() http.Handler {
	ensureInitialized()
	return otelHandler()
}

// HTTPMetricsMiddleware wraps an HTTP handler to record metrics
// This now uses OpenTelemetry under the hood, but maintains backward compatibility
func HTTPMetricsMiddleware(next http.Handler) http.Handler {
	ensureInitialized()
	return otelHTTPMetricsMiddleware(next)
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// normalizeRoute normalizes route paths for metrics by matching against registered routes/templates.
// Returns normalized template (e.g., "/api/v1/applications/:id/activate") or "unknown" for unrecognized patterns.
// Path should be r.URL.Path (no query parameters).
func normalizeRoute(path string) string {
	if path == "" || path == "/" {
		return "/"
	}

	parts := strings.Split(path, "/")
	// Remove empty first element from split
	if len(parts) > 0 && parts[0] == "" {
		parts = parts[1:]
	}

	if len(parts) == 0 {
		return "/"
	}

	fullPath := "/" + strings.Join(parts, "/")

	routesMu.RLock()
	defer routesMu.RUnlock()
	// Check exact static routes first (O(1) lookup)
	if exactMatch, exists := routes[fullPath]; exists && exactMatch {
		return fullPath
	}

	// Match against registered templates
	for _, template := range routeTemplates {
		if matchesTemplate(fullPath, template, parts) {
			return template
		}
	}

	// Fallback: detect ID patterns for unregistered routes (checks all segments)
	if len(parts) == 1 {
		return "unknown"
	}

	if len(parts) == 2 {
		if looksLikeID(parts[1]) && !isCommonPathWord(parts[1]) {
			return "/" + parts[0] + "/:id"
		}
		return "unknown"
	}

	// For 3+ segments: check for ID patterns (handles IDs in middle)
	if len(parts) >= 3 {
		normalized := make([]string, len(parts))
		copy(normalized, parts)
		idFound := false

		// Check segments, skipping short prefix segments (e.g., "api", "v1") and common path words
		for i, part := range parts {
			if i < 2 && len(part) <= 3 {
				continue // Skip API versioning segments
			}
			if looksLikeID(part) && !isCommonPathWord(part) {
				normalized[i] = ":id"
				idFound = true
			}
		}

		// Return normalized path if ID found and path length is reasonable (max 6 segments)
		if idFound && len(parts) <= 6 {
			return "/" + strings.Join(normalized, "/")
		}
		return "unknown"
	}

	return "unknown"
}

// matchesTemplate checks if a path matches a route template. Supports :id and {id} placeholders.
func matchesTemplate(path, template string, pathParts []string) bool {
	templateParts := strings.Split(template, "/")
	if len(templateParts) > 0 && templateParts[0] == "" {
		templateParts = templateParts[1:]
	}

	if len(pathParts) != len(templateParts) {
		return false
	}

	for i := 0; i < len(pathParts); i++ {
		// Support both :id and {id} syntax (normalized to :id during registration)
		if templateParts[i] == ":id" || templateParts[i] == "{id}" {
			continue // Placeholder - skip validation
		}
		if pathParts[i] != templateParts[i] {
			return false
		}
	}

	return true
}

// looksLikeID checks if a string looks like a dynamic ID (UUID, numeric, email, version string, or alphanumeric)
func looksLikeID(s string) bool {
	if s == "" {
		return false
	}

	// Check for UUID-like patterns (e.g., "123e4567-e89b-12d3-a456-426614174000")
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	// Check for other IDs with separators that also contain numbers (e.g., "consent_abc123")
	if (strings.Contains(s, "_") || strings.Contains(s, "-")) && strings.ContainsAny(s, "0123456789") {
		return true
	}

	// Check for version strings (e.g., "v1.0.0", "2.3.1")
	if strings.Contains(s, ".") && len(s) >= 3 {
		return true
	}

	// Check if it's all numeric (e.g., "123")
	allNumeric := true
	for _, r := range s {
		if r < '0' || r > '9' {
			allNumeric = false
			break
		}
	}
	if allNumeric && len(s) > 0 {
		return true
	}

	// Check if it looks like an email (contains @)
	if strings.Contains(s, "@") {
		return true
	}

	// Check if it's alphanumeric (likely an ID) - reduced threshold from 10 to 4 chars
	if len(s) >= 4 {
		alphanumeric := true
		for _, r := range s {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
				alphanumeric = false
				break
			}
		}
		if alphanumeric {
			return true
		}
	}

	return false
}

// isCommonPathWord checks if a segment is a common path word (not an ID) to prevent false positives.
func isCommonPathWord(word string) bool {
	if len(word) <= 2 {
		return true
	}
	commonWords := map[string]bool{
		"api": true, "v1": true, "v2": true, "v3": true,
		"applications": true, "application": true, "users": true, "user": true,
		"consents": true, "consent": true, "schemas": true, "schema": true,
		"versions": true, "version": true, "activate": true, "deactivate": true,
		"profile": true, "profiles": true, "posts": true,
		"list": true, "create": true, "update": true, "delete": true,
		"get": true, "post": true, "put": true, "patch": true,
		"check": true, "admin": true,
	}
	return commonWords[strings.ToLower(word)]
}

// RecordExternalCall records an external service call
// This now uses OpenTelemetry under the hood, but maintains backward compatibility
func RecordExternalCall(target, operation string, duration time.Duration, err error) {
	ensureInitialized()
	otelRecordExternalCall(target, operation, duration, err)
}

// RecordBusinessEvent records a business event
// This now uses OpenTelemetry under the hood, but maintains backward compatibility
func RecordBusinessEvent(action, outcome string) {
	ensureInitialized()
	otelRecordBusinessEvent(action, outcome)
}
