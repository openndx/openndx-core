// Package policy is the Portal Backend's client for the Policy Decision Point
// (PDP). It owns the PDP request/response types the Portal Backend relies on,
// so the rest of the Portal Backend never depends on PDP internals.
package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client handles communication with the Policy Decision Point
type Client struct {
	// baseURL is the endpoint of the PDP
	baseURL string
	// HTTPClient is used to make requests to the PDP
	HTTPClient *http.Client
}

// NewClient creates a new instance of Client.
// The PDP is reached through a trusted API gateway, so no API key is required.
func NewClient(baseURL string) *Client {
	// Trim any trailing slash to avoid double slashes in constructed URLs.
	baseURL = strings.TrimSuffix(baseURL, "/")
	return &Client{
		baseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// CreatePolicyMetadata sends a request to create policy metadata in the PDP,
// parsing the field records out of a GraphQL SDL string.
func (c *Client) CreatePolicyMetadata(schemaId string, sdl string) (*PolicyMetadataCreateResponse, error) {
	policyRequest, err := newSDLParser().ParseSDLToPolicyRequest(schemaId, sdl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SDL: %w", err)
	}
	return c.createPolicyMetadata(policyRequest)
}

// CreatePolicyMetadataFromRecords sends a request to create policy metadata
// in the PDP from caller-supplied records directly, skipping SDL/GraphQL
// parsing entirely.
func (c *Client) CreatePolicyMetadataFromRecords(schemaId string, records []PolicyMetadataCreateRequestRecord) (*PolicyMetadataCreateResponse, error) {
	return c.createPolicyMetadata(&PolicyMetadataCreateRequest{
		SchemaID: schemaId,
		Records:  records,
	})
}

func (c *Client) createPolicyMetadata(policyRequest *PolicyMetadataCreateRequest) (*PolicyMetadataCreateResponse, error) {
	// Marshal request to JSON
	reqBody, err := json.Marshal(policyRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/policy/metadata", c.baseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Send request to PDP
	resp, err := c.HTTPClient.Do(httpReq)
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
	var response PolicyMetadataCreateResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	slog.Info("Successfully created policy metadata in PDP", "schemaId", policyRequest.SchemaID, "recordsCreated", len(response.Records))
	return &response, nil
}

// CreateAccessGrants grants an application access to fields in the PDP,
// renewing any active grant it already holds for a field.
func (c *Client) CreateAccessGrants(request AccessGrantCreateRequest) (*AccessGrantListResponse, error) {
	var response AccessGrantListResponse
	if err := c.doRequest(http.MethodPost, "/api/v1/policy/grants", request, http.StatusOK, &response); err != nil {
		return nil, err
	}

	slog.Info("Successfully granted access in PDP", "applicationId", request.ApplicationID,
		"consentBypass", request.ConsentBypass, "grants", len(response.Records))
	return &response, nil
}

// ListAccessGrants lists the PDP's access grants that match the filter.
func (c *Client) ListAccessGrants(filter AccessGrantFilter) (*AccessGrantListResponse, error) {
	query := url.Values{}
	if filter.ApplicationID != "" {
		query.Set("applicationId", filter.ApplicationID)
	}
	if filter.SchemaID != "" {
		query.Set("schemaId", filter.SchemaID)
	}
	if filter.Status != "" {
		query.Set("status", filter.Status)
	}
	if filter.ConsentBypass != nil {
		query.Set("consentBypass", strconv.FormatBool(*filter.ConsentBypass))
	}
	path := "/api/v1/policy/grants"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	var response AccessGrantListResponse
	if err := c.doRequest(http.MethodGet, path, nil, http.StatusOK, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// GetAccessGrant reads one access grant from the PDP.
func (c *Client) GetAccessGrant(id string) (*AccessGrant, error) {
	var response AccessGrant
	if err := c.doRequest(http.MethodGet, "/api/v1/policy/grants/"+url.PathEscape(id), nil, http.StatusOK, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// RevokeAccessGrant revokes one active access grant in the PDP.
func (c *Client) RevokeAccessGrant(id string) (*AccessGrant, error) {
	path := "/api/v1/policy/grants/" + url.PathEscape(id) + "/revoke"

	var response AccessGrant
	if err := c.doRequest(http.MethodPost, path, nil, http.StatusOK, &response); err != nil {
		return nil, err
	}

	slog.Info("Successfully revoked access grant in PDP", "id", id, "applicationId", response.ApplicationID)
	return &response, nil
}

// Error is returned when the PDP responds with an unexpected status code,
// so callers can map the PDP's status back onto their own response.
type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("PDP returned status %d: %s", e.StatusCode, e.Message)
}

// ListPolicyMetadata lists the policy metadata records of a schema in the PDP
func (c *Client) ListPolicyMetadata(schemaID string) (*PolicyMetadataListResponse, error) {
	path := "/api/v1/policy/metadata?schemaId=" + url.QueryEscape(schemaID)

	var response PolicyMetadataListResponse
	if err := c.doRequest(http.MethodGet, path, nil, http.StatusOK, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// PatchPolicyMetadata updates selected properties of one policy metadata record in the PDP
func (c *Client) PatchPolicyMetadata(id string, request *PolicyMetadataPatchRequest) (*PolicyMetadataResponse, error) {
	path := "/api/v1/policy/metadata/" + url.PathEscape(id)

	var response PolicyMetadataResponse
	if err := c.doRequest(http.MethodPatch, path, request, http.StatusOK, &response); err != nil {
		return nil, err
	}

	slog.Info("Successfully patched policy metadata in PDP", "id", id)
	return &response, nil
}

// DeletePolicyMetadata deletes one policy metadata record in the PDP
func (c *Client) DeletePolicyMetadata(id string) error {
	path := "/api/v1/policy/metadata/" + url.PathEscape(id)

	if err := c.doRequest(http.MethodDelete, path, nil, http.StatusNoContent, nil); err != nil {
		return err
	}

	slog.Info("Successfully deleted policy metadata in PDP", "id", id)
	return nil
}

// doRequest sends a JSON request to the PDP and decodes the response into out.
// A response with a status other than expectedStatus is returned as an *Error.
func (c *Client) doRequest(method, path string, body any, expectedStatus int, out any) error {
	var reqBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(payload)
	}

	httpReq, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(httpReq)
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
		return &Error{StatusCode: resp.StatusCode, Message: message}
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	return nil
}
