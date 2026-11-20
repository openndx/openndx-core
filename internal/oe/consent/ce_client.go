package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/openndx/openndx-core/internal/monitoring"
)

// CEServiceClient represents a client to interact with the Consent Engine service
type CEServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewCEServiceClient creates a new instance of CEServiceClient
func NewCEServiceClient(baseURL string) *CEServiceClient {
	return &CEServiceClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateConsent sends a request to create a new consent record
func (c *CEServiceClient) CreateConsent(ctx context.Context, request *CreateConsentRequest) (*ConsentResponseInternalView, error) {
	// Implementation of the method to send HTTP request to create consent
	requestBody, err := json.Marshal(request)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to marshal CreateConsentRequest", "error", err)
		return nil, err
	}

	slog.InfoContext(ctx, "Making Create Consent Request to Consent Engine", "url", c.baseURL+consentEndpointPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+consentEndpointPath, bytes.NewBuffer(requestBody))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create HTTP request for CreateConsent", "error", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Propagate traceID from context to header for audit correlation
	traceID := monitoring.GetTraceIDFromContext(ctx)
	if traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to send HTTP request for CreateConsent", "error", err)
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		var errorBody bytes.Buffer
		if _, err := errorBody.ReadFrom(resp.Body); err != nil {
			slog.ErrorContext(ctx, "Failed to read error response body", "error", err)
		}
		errorMsg := errorBody.String()
		slog.ErrorContext(ctx, "Failed to create consent", "status", resp.StatusCode, "response", errorMsg)
		return nil, fmt.Errorf("failed to create consent, status code: %d, response: %s", resp.StatusCode, errorMsg)
	}

	var consentResponse ConsentResponseInternalView
	if err := json.NewDecoder(resp.Body).Decode(&consentResponse); err != nil {
		slog.ErrorContext(ctx, "Failed to decode CreateConsent response", "error", err)
		return nil, err
	}

	return &consentResponse, nil
}
