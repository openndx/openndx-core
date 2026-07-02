package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/OpenDIF/opendif-core/exchange/shared/monitoring"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
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
		logger.Log.Error("Failed to marshal CreateConsentRequest", "error", err)
		return nil, err
	}

	logger.Log.Info("Making Create Consent Request to Consent Engine", "url", c.baseURL+consentEndpointPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+consentEndpointPath, bytes.NewBuffer(requestBody))
	if err != nil {
		logger.Log.Error("Failed to create HTTP request for CreateConsent", "error", err)
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
		logger.Log.Error("Failed to send HTTP request for CreateConsent", "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errorBody bytes.Buffer
		if _, err := errorBody.ReadFrom(resp.Body); err != nil {
			logger.Log.Error("Failed to read error response body", "error", err)
		}
		errorMsg := errorBody.String()
		logger.Log.Error("Failed to create consent", "status", resp.StatusCode, "response", errorMsg)
		return nil, fmt.Errorf("failed to create consent, status code: %d, response: %s", resp.StatusCode, errorMsg)
	}

	var consentResponse ConsentResponseInternalView
	if err := json.NewDecoder(resp.Body).Decode(&consentResponse); err != nil {
		logger.Log.Error("Failed to decode CreateConsent response", "error", err)
		return nil, err
	}

	return &consentResponse, nil
}
