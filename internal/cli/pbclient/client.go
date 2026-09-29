// Package pbclient is a minimal Portal Backend API client for CLI management commands.
package pbclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openndx/openndx-core/internal/pb/models"
)

// Client calls the Portal Backend management API using a bearer token.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates a Portal Backend client for the given base URL and bearer token.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// doJSON sends a request to path with reqBody marshaled as the JSON body (or
// no body, if reqBody is nil), and returns the raw response body on success.
func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any) ([]byte, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if reqBody != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to reach portal backend: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("portal backend returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// CreateApplication calls POST /api/v1/applications.
func (c *Client) CreateApplication(ctx context.Context, req *models.CreateApplicationRequest) (*models.ApplicationResponse, error) {
	body, err := c.doJSON(ctx, http.MethodPost, "/api/v1/applications", req)
	if err != nil {
		return nil, err
	}
	var app models.ApplicationResponse
	if err := json.Unmarshal(body, &app); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &app, nil
}

// CreateSchema calls POST /api/v1/schemas.
func (c *Client) CreateSchema(ctx context.Context, req *models.CreateSchemaRequest) (*models.SchemaResponse, error) {
	body, err := c.doJSON(ctx, http.MethodPost, "/api/v1/schemas", req)
	if err != nil {
		return nil, err
	}
	var schema models.SchemaResponse
	if err := json.Unmarshal(body, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &schema, nil
}

// CreateMember calls POST /api/v1/members.
func (c *Client) CreateMember(ctx context.Context, req *models.CreateMemberRequest) (*models.MemberResponse, error) {
	body, err := c.doJSON(ctx, http.MethodPost, "/api/v1/members", req)
	if err != nil {
		return nil, err
	}
	var member models.MemberResponse
	if err := json.Unmarshal(body, &member); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &member, nil
}

// GetApplication calls GET /api/v1/applications/{applicationId}.
func (c *Client) GetApplication(ctx context.Context, applicationID string) (*models.ApplicationResponse, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/api/v1/applications/"+url.PathEscape(applicationID), nil)
	if err != nil {
		return nil, err
	}
	var app models.ApplicationResponse
	if err := json.Unmarshal(body, &app); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &app, nil
}

// ApplicationCollection is the GET /api/v1/applications response envelope.
type ApplicationCollection struct {
	Items []models.ApplicationResponse `json:"items"`
	Count int                          `json:"count"`
}

// ListApplications calls GET /api/v1/applications, optionally filtered to one
// member's applications.
func (c *Client) ListApplications(ctx context.Context, memberID *string) (*ApplicationCollection, error) {
	path := "/api/v1/applications"
	if memberID != nil && *memberID != "" {
		path += "?memberId=" + url.QueryEscape(*memberID)
	}

	body, err := c.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var collection ApplicationCollection
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &collection, nil
}

// UpdateApplicationPolicy calls PUT /api/v1/applications/{applicationId}/policy
// to replace an existing application's allow-list.
func (c *Client) UpdateApplicationPolicy(ctx context.Context, applicationID string, req *models.UpdateApplicationPolicyRequest) (*models.ApplicationResponse, error) {
	body, err := c.doJSON(ctx, http.MethodPut, "/api/v1/applications/"+url.PathEscape(applicationID)+"/policy", req)
	if err != nil {
		return nil, err
	}
	var app models.ApplicationResponse
	if err := json.Unmarshal(body, &app); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &app, nil
}
