package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// AuditLogsEndpoint is the API endpoint for creating audit logs
	AuditLogsEndpoint = "/api/audit-logs"
	// DefaultHTTPTimeout is the default timeout for HTTP requests to the audit service
	DefaultHTTPTimeout = 10 * time.Second
)

// Client is a client for sending audit events to the audit service
type Client struct {
	baseURL    string
	httpClient *http.Client
	enabled    bool
}

// NewClient creates a new audit client
// Audit can be disabled by:
//   - Setting ENABLE_AUDIT=false environment variable
//   - Providing an empty baseURL
//
// When disabled, all LogEvent calls will be no-ops.
func NewClient(baseURL string) *Client {
	enabled := isAuditEnabled(baseURL)

	if !enabled {
		slog.Info("Audit client disabled",
			"reason", "ENABLE_AUDIT=false or audit service URL not configured",
			"impact", "Services will continue running but audit events will not be logged")
		return &Client{
			baseURL:    "",
			httpClient: nil,
			enabled:    false,
		}
	}

	slog.Info("Audit client initialized", "baseURL", baseURL)
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
			},
		},
		enabled: true,
	}
}

// IsEnabled returns whether the audit client is enabled
func (c *Client) IsEnabled() bool {
	return c.enabled
}

// LogEvent sends an audit event to the audit service asynchronously (fire-and-forget)
// This function returns immediately and logs the event in a background goroutine.
func (c *Client) LogEvent(ctx context.Context, event *AuditLogRequest) {
	// Skip if audit client is not enabled
	if !c.enabled || c.httpClient == nil {
		return
	}

	// Log asynchronously (fire-and-forget). Detach from the request's cancellation
	// and deadline via context.WithoutCancel so the audit call completes even after
	// the originating request finishes, while preserving context values (e.g. tracing metadata).
	go c.logEvent(context.WithoutCancel(ctx), event)
}

// logEvent sends the audit event to the audit service API
func (c *Client) logEvent(ctx context.Context, event *AuditLogRequest) {
	if c.httpClient == nil {
		return
	}

	payloadBytes, err := json.Marshal(event)
	if err != nil {
		slog.Error("Failed to marshal audit request", "error", err)
		return
	}

	// Construct URL safely
	endpointURL, err := url.JoinPath(c.baseURL, AuditLogsEndpoint)
	if err != nil {
		slog.Error("Failed to construct audit service URL", "error", err, "baseURL", c.baseURL)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		slog.Error("Failed to create audit request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("Failed to send audit request", "error", err)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("Failed to close audit response body", "error", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			slog.Error("Audit service returned non-201 status and failed to read body",
				"status", resp.StatusCode, "readError", readErr)
		} else {
			slog.Error("Audit service returned non-201 status",
				"status", resp.StatusCode, "body", string(bodyBytes))
		}
		return
	}

	slog.Info("Audit event logged successfully",
		"eventType", event.EventType,
		"actorType", event.ActorType,
		"actorId", event.ActorID,
		"targetType", event.TargetType,
		"status", event.Status,
		"additionalMetadata", string(event.AdditionalMetadata))
}

// isAuditEnabled checks if audit logging is enabled via environment variable
// Audit is enabled by default unless explicitly disabled via ENABLE_AUDIT=false
// or if baseURL is empty
func isAuditEnabled(baseURL string) bool {
	// If URL is explicitly empty, audit is disabled
	if baseURL == "" {
		return false
	}

	// Check ENABLE_AUDIT environment variable (default: true)
	enableAudit := os.Getenv("ENABLE_AUDIT")
	if enableAudit == "" {
		// Default to enabled if URL is provided
		return true
	}

	// Parse boolean value (case-insensitive)
	enableAuditLower := strings.ToLower(strings.TrimSpace(enableAudit))
	return enableAuditLower == "true" || enableAuditLower == "1" || enableAuditLower == "yes"
}
