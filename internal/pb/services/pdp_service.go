package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openndx/openndx-core/internal/pb/models"
	"github.com/openndx/openndx-core/internal/pb/utils"
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

// CreatePolicyMetadata sends a request to create policy metadata in the PDP,
// parsing the field records out of a GraphQL SDL string.
func (s *PDPService) CreatePolicyMetadata(schemaId string, sdl string) (*models.PolicyMetadataCreateResponse, error) {
	handler := utils.NewGraphQLHandler()
	policyRequest, err := handler.ParseSDLToPolicyRequest(schemaId, sdl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SDL: %w", err)
	}
	return s.createPolicyMetadata(policyRequest)
}

// CreatePolicyMetadataFromRecords sends a request to create policy metadata
// in the PDP from caller-supplied records directly, skipping SDL/GraphQL
// parsing entirely.
func (s *PDPService) CreatePolicyMetadataFromRecords(schemaId string, records []models.PolicyMetadataCreateRequestRecord) (*models.PolicyMetadataCreateResponse, error) {
	return s.createPolicyMetadata(&models.PolicyMetadataCreateRequest{
		SchemaID: schemaId,
		Records:  records,
	})
}

func (s *PDPService) createPolicyMetadata(policyRequest *models.PolicyMetadataCreateRequest) (*models.PolicyMetadataCreateResponse, error) {
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

	// Check status code. The PDP's CreatePolicyMetadata handler responds 201
	// Created (it's a creation endpoint), not 200 - see
	// internal/pdp/handler/handler.go.
	if resp.StatusCode != http.StatusCreated {
		slog.Error("PDP returned error", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("PDP returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var response models.PolicyMetadataCreateResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	slog.Info("Successfully created policy metadata in PDP", "schemaId", policyRequest.SchemaID, "recordsCreated", len(response.Records))
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

// PDPError is returned when the PDP responds with an unexpected status code,
// so callers can map the PDP's status back onto their own response.
type PDPError struct {
	StatusCode int
	Message    string
}

func (e *PDPError) Error() string {
	return fmt.Sprintf("PDP returned status %d: %s", e.StatusCode, e.Message)
}

// ListPolicyMetadata lists the policy metadata records of a schema in the PDP
func (s *PDPService) ListPolicyMetadata(schemaID string) (*models.PolicyMetadataListResponse, error) {
	path := "/api/v1/policy/metadata?schemaId=" + url.QueryEscape(schemaID)

	var response models.PolicyMetadataListResponse
	if err := s.doRequest(http.MethodGet, path, nil, http.StatusOK, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// PatchPolicyMetadata updates selected properties of one policy metadata record in the PDP
func (s *PDPService) PatchPolicyMetadata(id string, request *models.PolicyMetadataPatchRequest) (*models.PolicyMetadataResponse, error) {
	path := "/api/v1/policy/metadata/" + url.PathEscape(id)

	var response models.PolicyMetadataResponse
	if err := s.doRequest(http.MethodPatch, path, request, http.StatusOK, &response); err != nil {
		return nil, err
	}

	slog.Info("Successfully patched policy metadata in PDP", "id", id)
	return &response, nil
}

// DeletePolicyMetadata deletes one policy metadata record in the PDP
func (s *PDPService) DeletePolicyMetadata(id string) error {
	path := "/api/v1/policy/metadata/" + url.PathEscape(id)

	if err := s.doRequest(http.MethodDelete, path, nil, http.StatusNoContent, nil); err != nil {
		return err
	}

	slog.Info("Successfully deleted policy metadata in PDP", "id", id)
	return nil
}

// RevokeAllowListEntry removes one application from a field's allow-list in the PDP
func (s *PDPService) RevokeAllowListEntry(id string, applicationID string) error {
	path := fmt.Sprintf("/api/v1/policy/metadata/%s/allowlist/%s", url.PathEscape(id), url.PathEscape(applicationID))

	if err := s.doRequest(http.MethodDelete, path, nil, http.StatusNoContent, nil); err != nil {
		return err
	}

	slog.Info("Successfully revoked allow-list entry in PDP", "id", id, "applicationId", applicationID)
	return nil
}

// doRequest sends a JSON request to the PDP and decodes the response into out.
// A response with a status other than expectedStatus is returned as a *PDPError.
func (s *PDPService) doRequest(method, path string, body any, expectedStatus int, out any) error {
	var reqBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(payload)
	}

	httpReq, err := http.NewRequest(method, s.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request to PDP: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			slog.Error("failed to close response body", "error", err)
		}
	}(resp.Body)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != expectedStatus {
		slog.Error("PDP returned error", "method", method, "path", path, "status", resp.StatusCode, "body", string(respBody))

		message := strings.TrimSpace(string(respBody))
		var errorResponse struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errorResponse) == nil && errorResponse.Error != "" {
			message = errorResponse.Error
		}
		return &PDPError{StatusCode: resp.StatusCode, Message: message}
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	return nil
}
