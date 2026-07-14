package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/shared/monitoring"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
)

// PdpClient represents a client to interact with the Policy Decision Point service
type PdpClient struct {
	httpClient *http.Client
	baseUrl    string
	tracker    monitoring.Tracker
}

// NewPdpClient creates a new instance of PdpClient
func NewPdpClient(baseUrl string, tracker monitoring.Tracker) *PdpClient {
	if tracker == nil {
		tracker = monitoring.NewNoOpTracker()
	}
	return &PdpClient{
		httpClient: &http.Client{
			Timeout: time.Second * 10,
		},
		baseUrl: baseUrl,
		tracker: tracker,
	}
}

// MakePdpRequest sends a request to get a policy decision
func (p *PdpClient) MakePdpRequest(ctx context.Context, request *PdpRequest) (pdpResponse *PdpResponse, err error) {
	// Implement the logic to make a PDP request using p.httpClient
	requestBody, err := json.Marshal(request)
	if err != nil {
		// handle error
		logger.Log.Error("Failed to marshal PDP request", "error", err)
		return nil, err
	}

	// log the json request body
	logger.Log.Info("PDP Request Body", "body", string(requestBody))

	// Create request with context for cancellation and timeout support
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseUrl+policyDecisionEndpointPath, bytes.NewReader(requestBody))
	if err != nil {
		logger.Log.Error("Failed to create PDP request", "error", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// Propagate traceID from context to header for audit correlation
	traceID := monitoring.GetTraceIDFromContext(ctx)
	if traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}

	start := time.Now()
	defer func() {
		p.tracker.RecordExternalCall("policy-decision-point", "decide", time.Since(start), err)
	}()

	response, err := p.httpClient.Do(req)
	if err != nil {
		// handle error
		logger.Log.Error("Failed to make PDP request", "error", err)
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		var errorBody bytes.Buffer
		if _, readErr := errorBody.ReadFrom(response.Body); readErr != nil {
			logger.Log.Error("Failed to read error response body", "error", readErr)
		}
		errorMsg := errorBody.String()
		logger.Log.Error("PDP request failed", "status", response.StatusCode, "response", errorMsg)
		err = fmt.Errorf("PDP request failed, status code: %d, response: %s", response.StatusCode, errorMsg)
		return nil, err
	}

	pdpResponse = &PdpResponse{}
	err = json.NewDecoder(response.Body).Decode(pdpResponse)
	if err != nil {
		// handle error
		logger.Log.Error("Failed to decode PDP response", "error", err)
		return nil, err
	}

	return pdpResponse, nil
}
