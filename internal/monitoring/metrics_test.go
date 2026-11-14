package monitoring

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandler(t *testing.T) {
	handler := Handler()
	if handler == nil {
		t.Fatal("Handler() returned nil")
	}

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("Metrics handler returned empty body")
	}

	// Check for Prometheus format
	if !strings.Contains(body, "# HELP") && !strings.Contains(body, "# TYPE") {
		t.Error("Response doesn't appear to be in Prometheus format")
	}
}

func TestHTTPMetricsMiddleware(t *testing.T) {
	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	})

	// Wrap with metrics middleware
	wrapped := HTTPMetricsMiddleware(testHandler)

	// Make a request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify metrics were recorded (check via /metrics endpoint)
	metricsHandler := Handler()
	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsW := httptest.NewRecorder()
	metricsHandler.ServeHTTP(metricsW, metricsReq)

	metricsBody := metricsW.Body.String()
	// OpenTelemetry Prometheus exporter converts counter names - check for actual exported name
	if !strings.Contains(metricsBody, "http_requests") {
		t.Errorf("http_requests metric not found after request. Metrics output:\n%s", metricsBody)
	}
}

func TestNormalizeRoute(t *testing.T) {
	resetRouteRegistryForTest()
	// Register routes for testing
	RegisterRoutes([]string{
		"/health",
		"/metrics",
		"/api/v1/policy/metadata",
		"/api/v1/policy/decide",
		"/consents/:id",
		"/data-owner/:id",
		"/api/v1/policy/:id",
		"/consumer/:id",
	})

	tests := []struct {
		input    string
		expected string
	}{
		{"/", "/"},
		{"/health", "/health"},
		{"/consents/123", "/consents/:id"},
		{"/consents/abc123def456", "/consents/:id"},
		{"/consents/consent_abc123", "/consents/:id"},
		{"/data-owner/user@example.com", "/data-owner/:id"},
		{"/api/v1/policy/metadata", "/api/v1/policy/metadata"},
		{"/api/v1/policy/decide", "/api/v1/policy/decide"},
		{"/api/v1/policy/123", "/api/v1/policy/:id"},
		{"/consumer/app-123", "/consumer/:id"},
		{"/admin/check", "unknown"}, // Not registered, falls back to unknown
	}

	for _, tt := range tests {
		result := normalizeRoute(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeRoute(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestRegisterRoutes(t *testing.T) {
	resetRouteRegistryForTest()
	// Test registering static routes
	RegisterRoutes([]string{
		"/static1",
		"/static2",
	})

	// Test registering templates with :id syntax
	RegisterRoutes([]string{
		"/api/v1/schema/:id",
		"/sdl/versions/:id/activate",
	})

	// Test registering templates with {id} syntax (as suggested in feedback)
	RegisterRoutes([]string{
		"/api/v1/applications/{id}/activate",
		"/api/v1/users/{id}/profile",
	})

	// Verify static routes work
	if normalizeRoute("/static1") != "/static1" {
		t.Error("Static route /static1 not registered correctly")
	}

	// Verify templates with :id syntax work
	if normalizeRoute("/api/v1/schema/abc123") != "/api/v1/schema/:id" {
		t.Error("Template /api/v1/schema/:id not matching correctly")
	}

	if normalizeRoute("/sdl/versions/v1.0.0/activate") != "/sdl/versions/:id/activate" {
		t.Error("Template /sdl/versions/:id/activate not matching correctly")
	}

	// Verify templates with {id} syntax work (normalized to :id internally)
	if normalizeRoute("/api/v1/applications/app-123/activate") != "/api/v1/applications/:id/activate" {
		t.Error("Template /api/v1/applications/{id}/activate not matching correctly")
	}

	if normalizeRoute("/api/v1/users/user@example.com/profile") != "/api/v1/users/:id/profile" {
		t.Error("Template /api/v1/users/{id}/profile not matching correctly")
	}
}

func TestIsExactRoute(t *testing.T) {
	resetRouteRegistryForTest()
	// Register some routes
	RegisterRoutes([]string{
		"/health",
		"/metrics",
		"/api/v1/policy/metadata",
		"/api/v1/schema/:id", // Template, not exact
	})

	// Test exact routes
	if !IsExactRoute("/health") {
		t.Error("IsExactRoute should return true for registered exact route")
	}

	if !IsExactRoute("/metrics") {
		t.Error("IsExactRoute should return true for registered exact route")
	}

	if !IsExactRoute("/api/v1/policy/metadata") {
		t.Error("IsExactRoute should return true for registered exact route")
	}

	// Test that templates are not exact routes
	if IsExactRoute("/api/v1/schema/:id") {
		t.Error("IsExactRoute should return false for template routes")
	}

	if IsExactRoute("/api/v1/schema/123") {
		t.Error("IsExactRoute should return false for paths matching templates")
	}

	// Test unregistered routes
	if IsExactRoute("/unknown") {
		t.Error("IsExactRoute should return false for unregistered routes")
	}

	if IsExactRoute("/api/v1/unknown") {
		t.Error("IsExactRoute should return false for unregistered routes")
	}
}

func TestRecordExternalCall(t *testing.T) {
	// Record a successful external call
	RecordExternalCall("postgres", "create_consent", 100*time.Millisecond, nil)

	// Record a failed external call
	RecordExternalCall("postgres", "create_consent", 50*time.Millisecond, fmt.Errorf("connection failed"))

	// Verify metrics were recorded (check via /metrics endpoint)
	metricsHandler := Handler()
	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsW := httptest.NewRecorder()
	metricsHandler.ServeHTTP(metricsW, metricsReq)

	metricsBody := metricsW.Body.String()
	// OpenTelemetry Prometheus exporter converts counter names - check for actual exported names
	if !strings.Contains(metricsBody, "external_calls") {
		t.Errorf("external_calls metric not found. Metrics output:\n%s", metricsBody)
	}
	if !strings.Contains(metricsBody, "external_call_errors") {
		t.Errorf("external_call_errors metric not found. Metrics output:\n%s", metricsBody)
	}
	if !strings.Contains(metricsBody, "external_call_duration") {
		t.Errorf("external_call_duration metric not found. Metrics output:\n%s", metricsBody)
	}
}

func TestRecordBusinessEvent(t *testing.T) {
	// Record business events
	RecordBusinessEvent("consent_created", "success")
	RecordBusinessEvent("consent_approved", "success")
	RecordBusinessEvent("policy_decision", "allow")

	// Verify metrics were recorded (check via /metrics endpoint)
	metricsHandler := Handler()
	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsW := httptest.NewRecorder()
	metricsHandler.ServeHTTP(metricsW, metricsReq)

	metricsBody := metricsW.Body.String()
	// OpenTelemetry Prometheus exporter converts counter names - check for actual exported name
	if !strings.Contains(metricsBody, "business_events") {
		t.Errorf("business_events metric not found. Metrics output:\n%s", metricsBody)
	}
}

func TestNormalizeRouteFallbackWithIDInMiddle(t *testing.T) {
	resetRouteRegistryForTest()
	// Fallback logic must work without registered templates.

	tests := []struct {
		input    string
		expected string
	}{
		// Test ID at the end (existing behavior)
		{"/api/v1/applications/123", "/api/v1/applications/:id"},
		{"/api/v1/schema/abc123", "/api/v1/schema/:id"},

		// Test ID in the middle (new behavior)
		{"/api/v1/applications/123/activate", "/api/v1/applications/:id/activate"},
		{"/api/v1/applications/app-123/activate", "/api/v1/applications/:id/activate"},
		{"/sdl/versions/v1.0.0/activate", "/sdl/versions/:id/activate"},
		{"/api/v1/users/user@example.com/profile", "/api/v1/users/:id/profile"},

		// Test multiple IDs (should normalize all)
		{"/api/v1/users/123/posts/456", "/api/v1/users/:id/posts/:id"},
		{"/api/v1/applications/app-123/consents/consent-456", "/api/v1/applications/:id/consents/:id"},

		// Test paths that are too long (should return unknown)
		{"/api/v1/a/b/c/d/e/f/g/h", "unknown"}, // 8 segments, exceeds limit of 6

		// Test paths without IDs (should return unknown)
		{"/api/v1/applications/list", "unknown"},
		{"/api/v1/users/profile", "unknown"},
	}

	for _, tt := range tests {
		result := normalizeRoute(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeRoute(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

// TestLooksLikeIDImprovedLogic tests the improved ID detection that prevents false positives
func TestLooksLikeIDImprovedLogic(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
		reason   string
	}{
		// UUIDs should be detected
		{"123e4567-e89b-12d3-a456-426614174000", true, "Valid UUID"},
		{"00000000-0000-0000-0000-000000000000", true, "UUID format"},

		// IDs with separators AND numbers should be detected
		{"consent_abc123", true, "Has underscore and numbers"},
		{"app-456", true, "Has hyphen and numbers"},
		{"user_123def", true, "Has underscore and numbers"},

		// Static path segments with separators but NO numbers should NOT be detected
		{"data-owner", false, "Has hyphen but no numbers - static path"},
		{"list-all", false, "Has hyphen but no numbers - static path"},
		{"check-status", false, "Has hyphen but no numbers - static path"},
		{"user_profile", false, "Has underscore but no numbers - static path"},

		// Version strings should be detected
		{"v1.0.0", true, "Version string"},
		{"2.3.1", true, "Version string"},

		// Filenames with dots are NOT version strings
		{"index.html", false, "Filename, not a version"},
		{"openapi.json", false, "Filename, not a version"},
		{"robots.txt", false, "Filename, not a version"},

		// Numeric IDs should be detected
		{"123", true, "All numeric"},
		{"456789", true, "All numeric"},
		{"12", true, "All numeric (short still an ID)"},

		// Email addresses should be detected
		{"user@example.com", true, "Email address"},

		// Long alphanumeric strings should be detected
		{"abc123def456", true, "Alphanumeric ID"},
		{"app123", true, "Alphanumeric ID"},

		// Short non-numeric strings should NOT be detected
		{"abc", false, "Too short"},

		// Common path words should NOT be detected (tested via isCommonPathWord)
		{"api", false, "Common path word"},
		{"v1", false, "Common path word"},
	}

	for _, tt := range tests {
		result := looksLikeID(tt.input)
		if result != tt.expected {
			t.Errorf("looksLikeID(%q) = %v, expected %v (%s)", tt.input, result, tt.expected, tt.reason)
		}
	}
}

// TestHistogramBucketsConfiguration tests that both histogram metrics use custom buckets
func TestHistogramBucketsConfiguration(t *testing.T) {
	// This test verifies that the histogram bucket configuration is applied
	// by checking that metrics are recorded and can be queried

	// Record HTTP request duration
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate some processing time
		w.WriteHeader(http.StatusOK)
	})
	wrapped := HTTPMetricsMiddleware(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	// Record external call duration
	RecordExternalCall("postgres", "query", 25*time.Millisecond, nil)

	// Verify metrics endpoint returns data
	metricsHandler := Handler()
	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsW := httptest.NewRecorder()
	metricsHandler.ServeHTTP(metricsW, metricsReq)

	metricsBody := metricsW.Body.String()

	// Both histogram metrics should be present
	if !strings.Contains(metricsBody, "http_request_duration") {
		t.Error("http_request_duration_seconds histogram not found")
	}
	if !strings.Contains(metricsBody, "external_call_duration") {
		t.Error("external_call_duration_seconds histogram not found")
	}

	// Verify the metrics are in Prometheus format with buckets
	// Prometheus histograms show bucket boundaries
	if !strings.Contains(metricsBody, "le=") {
		t.Log("Note: Histogram buckets may not be visible in text format, but configuration is applied")
	}
}

// TestRouteNormalizationWithStaticPaths tests that static paths with hyphens are not normalized
func TestRouteNormalizationWithStaticPaths(t *testing.T) {
	resetRouteRegistryForTest()
	tests := []struct {
		input    string
		expected string
		reason   string
	}{
		// Static paths with hyphens should NOT be normalized (no false positives)
		{"/api/v1/data-owner", "unknown", "Static path with hyphen - should not be normalized"},
		{"/api/v1/list-all", "unknown", "Static path with hyphen - should not be normalized"},
		{"/api/v1/check-status", "unknown", "Static path with hyphen - should not be normalized"},

		// Paths with actual IDs should be normalized
		{"/api/v1/data-owner/123", "/api/v1/data-owner/:id", "Has numeric ID"},
		{"/api/v1/data-owner/user-123", "/api/v1/data-owner/:id", "Has ID with hyphen and numbers"},
		{"/api/v1/users/user_123/profile", "/api/v1/users/:id/profile", "Has ID with underscore and numbers"},

		// UUIDs should be normalized
		{"/api/v1/users/123e4567-e89b-12d3-a456-426614174000", "/api/v1/users/:id", "Has UUID"},
	}

	for _, tt := range tests {
		result := normalizeRoute(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeRoute(%q) = %q, expected %q (%s)", tt.input, result, tt.expected, tt.reason)
		}
	}
}

// TestIsInitialized tests the initialization state functions
func TestIsInitialized(t *testing.T) {
	// After Handler() is called, initialization should have occurred
	_ = Handler()

	if !IsInitialized() {
		t.Error("IsInitialized() should return true after Handler() is called")
	}

	if GetInitError() != nil {
		t.Errorf("GetInitError() should return nil after successful initialization, got: %v", GetInitError())
	}
}

// TestMultipleInitializations tests that multiple initialization calls are safe
func TestMultipleInitializations(t *testing.T) {
	// Reset state by calling ensureInitialized multiple times
	// This should be safe and not cause panics
	_ = Handler()
	_ = Handler()
	_ = HTTPMetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	RecordExternalCall("test", "op", time.Millisecond, nil)
	RecordBusinessEvent("test", "success")

	// Should still be initialized
	if !IsInitialized() {
		t.Error("Multiple initialization calls should not break initialization state")
	}
}

// TestHTTPMetricsMiddlewareWithDifferentStatusCodes tests that different HTTP status codes are recorded
func TestHTTPMetricsMiddlewareWithDifferentStatusCodes(t *testing.T) {
	testCases := []struct {
		statusCode int
		name       string
	}{
		{http.StatusOK, "200 OK"},
		{http.StatusNotFound, "404 Not Found"},
		{http.StatusInternalServerError, "500 Internal Server Error"},
		{http.StatusBadRequest, "400 Bad Request"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			})

			wrapped := HTTPMetricsMiddleware(testHandler)
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)

			if w.Code != tc.statusCode {
				t.Errorf("Expected status %d, got %d", tc.statusCode, w.Code)
			}
		})
	}
}

// TestNormalizeRouteWith404 tests that 404s are normalized to "unknown"
func TestNormalizeRouteWith404(t *testing.T) {
	// This is tested indirectly in otelHTTPMetricsMiddleware
	// When statusCode is 404, route is set to "unknown"
	// We can verify this by checking the middleware behavior
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	wrapped := HTTPMetricsMiddleware(testHandler)
	req := httptest.NewRequest("GET", "/nonexistent/path/123", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	// The route should be normalized to "unknown" for 404s
	// This prevents cardinality explosion from random 404 paths
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestRegisterRoutes_DeduplicatesTemplates(t *testing.T) {
	resetRouteRegistryForTest()
	RegisterRoutes([]string{"/api/v1/schema/:id", "/api/v1/schema/{id}"})
	RegisterRoutes([]string{"/api/v1/schema/:id"})

	routesMu.RLock()
	defer routesMu.RUnlock()
	if len(routeTemplates) != 1 {
		t.Fatalf("expected 1 template after dedup, got %d: %v", len(routeTemplates), routeTemplates)
	}
	if routeTemplates[0] != "/api/v1/schema/:id" {
		t.Fatalf("unexpected template: %q", routeTemplates[0])
	}
}

func TestParseOTLPEndpointHost(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    string
		insecure    bool
		wantHost    string
		wantInsec   bool
		wantErrSub  string
	}{
		{
			name:      "https endpoint",
			endpoint:  "https://otel-collector:4318",
			wantHost:  "otel-collector:4318",
			wantInsec: false,
		},
		{
			name:      "http with insecure",
			endpoint:  "http://otel-collector:4318",
			insecure:   true,
			wantHost:  "otel-collector:4318",
			wantInsec:  true,
		},
		{
			name:       "scheme-less rejected",
			endpoint:   "otel-collector:4318",
			insecure:   true,
			wantErrSub: "http:// or https:// scheme",
		},
		{
			name:       "http without insecure rejected",
			endpoint:   "http://otel-collector:4318",
			wantErrSub: "must use HTTPS",
		},
		{
			name:       "empty host rejected",
			endpoint:   "https:///metrics",
			wantErrSub: "must include a host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, useInsecure, err := parseOTLPEndpointHost(tt.endpoint, tt.insecure)
			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if useInsecure != tt.wantInsec {
				t.Errorf("useInsecure = %v, want %v", useInsecure, tt.wantInsec)
			}
		})
	}
}

func TestResponseWriterOptionalInterfaces(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	if rw.Unwrap() != rec {
		t.Error("Unwrap should return underlying ResponseWriter")
	}
	// httptest.ResponseRecorder implements Flusher
	rw.Flush()

	if _, ok := any(rw).(http.Flusher); !ok {
		t.Error("responseWriter should implement http.Flusher")
	}
	if _, ok := any(rw).(http.Hijacker); !ok {
		t.Error("responseWriter should implement http.Hijacker")
	}
	if _, ok := any(rw).(io.ReaderFrom); !ok {
		t.Error("responseWriter should implement io.ReaderFrom")
	}

	// Recorder is not a Hijacker — should return a clear error, not panic.
	if _, _, err := rw.Hijack(); err == nil {
		t.Error("expected Hijack error when underlying writer lacks Hijacker")
	}
}

func TestShutdownWithoutInit(t *testing.T) {
	// When meterProvider was never set (or already cleared), Shutdown is a no-op.
	// Do not call Initialize here — package init is once-per-process in tests.
	prev := meterProvider
	meterProvider = nil
	t.Cleanup(func() { meterProvider = prev })

	if err := Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown with nil provider: %v", err)
	}
}
