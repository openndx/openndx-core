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

	"github.com/openndx/openndx-core/internal/pb/application"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/openndx/openndx-core/internal/pb/schema"
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
func (c *Client) CreateApplication(ctx context.Context, req *application.CreateApplicationRequest) (*application.ApplicationResponse, error) {
	body, err := c.doJSON(ctx, http.MethodPost, "/api/v1/applications", req)
	if err != nil {
		return nil, err
	}
	var app application.ApplicationResponse
	if err := json.Unmarshal(body, &app); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &app, nil
}

// CreateSchema calls POST /api/v1/schemas.
func (c *Client) CreateSchema(ctx context.Context, req *schema.CreateSchemaRequest) (*schema.SchemaResponse, error) {
	body, err := c.doJSON(ctx, http.MethodPost, "/api/v1/schemas", req)
	if err != nil {
		return nil, err
	}
	var resp schema.SchemaResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// CreateMember calls POST /api/v1/members.
func (c *Client) CreateMember(ctx context.Context, req *member.CreateMemberRequest) (*member.MemberResponse, error) {
	body, err := c.doJSON(ctx, http.MethodPost, "/api/v1/members", req)
	if err != nil {
		return nil, err
	}
	var resp member.MemberResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// GetMember calls GET /api/v1/members/{memberId}.
func (c *Client) GetMember(ctx context.Context, memberID string) (*member.MemberResponse, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/api/v1/members/"+url.PathEscape(memberID), nil)
	if err != nil {
		return nil, err
	}
	var resp member.MemberResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// MemberCollection is the GET /api/v1/members response envelope.
type MemberCollection struct {
	Items []member.MemberResponse `json:"items"`
	Count int                     `json:"count"`
}

// ListMembers calls GET /api/v1/members, optionally filtered to one
// organization unit's member.
func (c *Client) ListMembers(ctx context.Context, ouID *string) (*MemberCollection, error) {
	path := "/api/v1/members"
	if ouID != nil && *ouID != "" {
		path += "?ouId=" + url.QueryEscape(*ouID)
	}

	body, err := c.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var collection MemberCollection
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &collection, nil
}

// GetSchema calls GET /api/v1/schemas/{schemaId}.
func (c *Client) GetSchema(ctx context.Context, schemaID string) (*schema.SchemaResponse, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/api/v1/schemas/"+url.PathEscape(schemaID), nil)
	if err != nil {
		return nil, err
	}
	var resp schema.SchemaResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &resp, nil
}

// SchemaCollection is the GET /api/v1/schemas response envelope.
type SchemaCollection struct {
	Items []schema.SchemaResponse `json:"items"`
	Count int                     `json:"count"`
}

// ListSchemas calls GET /api/v1/schemas, optionally filtered to one member's
// schemas.
func (c *Client) ListSchemas(ctx context.Context, memberID *string) (*SchemaCollection, error) {
	path := "/api/v1/schemas"
	if memberID != nil && *memberID != "" {
		path += "?memberId=" + url.QueryEscape(*memberID)
	}

	body, err := c.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var collection SchemaCollection
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &collection, nil
}

// PolicyMetadataCollection is the GET /api/v1/schemas/{schemaId}/policy-metadata
// response envelope.
type PolicyMetadataCollection struct {
	Items []policy.PolicyMetadataResponse `json:"items"`
	Count int                             `json:"count"`
}

// ListSchemaPolicyMetadata calls GET /api/v1/schemas/{schemaId}/policy-metadata
// to list a schema's grantable fields.
func (c *Client) ListSchemaPolicyMetadata(ctx context.Context, schemaID string) (*PolicyMetadataCollection, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/api/v1/schemas/"+url.PathEscape(schemaID)+"/policy-metadata", nil)
	if err != nil {
		return nil, err
	}
	var collection PolicyMetadataCollection
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &collection, nil
}

// GetApplication calls GET /api/v1/applications/{applicationId}.
func (c *Client) GetApplication(ctx context.Context, applicationID string) (*application.ApplicationResponse, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/api/v1/applications/"+url.PathEscape(applicationID), nil)
	if err != nil {
		return nil, err
	}
	var app application.ApplicationResponse
	if err := json.Unmarshal(body, &app); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &app, nil
}

// ApplicationCollection is the GET /api/v1/applications response envelope.
type ApplicationCollection struct {
	Items []application.ApplicationResponse `json:"items"`
	Count int                               `json:"count"`
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
func (c *Client) UpdateApplicationPolicy(ctx context.Context, applicationID string, req *application.UpdateApplicationPolicyRequest) (*application.ApplicationResponse, error) {
	body, err := c.doJSON(ctx, http.MethodPut, "/api/v1/applications/"+url.PathEscape(applicationID)+"/policy", req)
	if err != nil {
		return nil, err
	}
	var app application.ApplicationResponse
	if err := json.Unmarshal(body, &app); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &app, nil
}
