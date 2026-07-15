package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/OpenNDX/openndx-core/portal-backend/v1/models"
	"github.com/OpenNDX/openndx-core/portal-backend/v1/utils"
)

// PDPService handles communication with the Policy Decision Point
type PDPService struct {
	// baseURL is the endpoint of the PDP
	baseURL string
	// HTTPClient is used to make requests to the PDP
	HTTPClient *http.Client
}

// NewPDPService creates a new instance of PDPService.
// The PDP is reached through a trusted API gateway, so no API key is required.
func NewPDPService(baseURL string) *PDPService {
	// Trim any trailing slash to avoid double slashes in constructed URLs.
	baseURL = strings.TrimSuffix(baseURL, "/")
	return &PDPService{
		baseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// CreatePolicyMetadata sends a request to create policy metadata in the PDP
func (s *PDPService) CreatePolicyMetadata(schemaId string, sdl string) (*models.PolicyMetadataCreateResponse, error) {
	// parse SDL and create policy metadata request
	handler := utils.NewGraphQLHandler()
	policyRequest, err := handler.ParseSDLToPolicyRequest(schemaId, sdl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SDL: %w", err)
	}

	// Marshal request to JSON
	reqBody, err := json.Marshal(policyRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/policy/metadata", s.baseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send request to PDP
	resp, err := s.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to PDP: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(resp.Body)

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		slog.Error("PDP returned error", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("PDP returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var response models.PolicyMetadataCreateResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	slog.Info("Successfully created policy metadata in PDP", "schemaId", schemaId, "recordsCreated", len(response.Records))
	return &response, nil
}

// UpdateAllowList sends a request to update the allow list in the PDP
func (s *PDPService) UpdateAllowList(request models.AllowListUpdateRequest) (*models.AllowListUpdateResponse, error) {
	// Marshal request to JSON
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/policy/update-allowlist", s.baseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	slog.Debug("Sending allow list update request to PDP", "url", url, "applicationId", request.ApplicationID)
	resp, err := s.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to PDP: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(resp.Body)

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		slog.Error("PDP returned error", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("PDP returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var response models.AllowListUpdateResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	slog.Info("Successfully updated allow list in PDP", "applicationId", request.ApplicationID, "recordsUpdated", len(response.Records))
	return &response, nil
}
