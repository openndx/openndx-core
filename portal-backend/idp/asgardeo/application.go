package asgardeo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/OpenNDX/openndx-core/portal-backend/idp"
)

type AsgardeoApplicationTemplate string

const (
	ApplicationTemplateIDM2M AsgardeoApplicationTemplate = "m2m-application"
)

type AsgardeoApplicationInfo struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ClientId    string `json:"clientId"`
}

type AsgardeoApplication struct {
	Name                         string                      `json:"name"`
	Description                  string                      `json:"description"`
	TemplateId                   AsgardeoApplicationTemplate `json:"templateId"`
	InboundProtocolConfiguration map[string]interface{}      `json:"inboundProtocolConfiguration"`
	AssociatedRoles              AssociatedRole              `json:"associatedRoles"`
}

type AssociatedRole struct {
	AllowedAudience string   `json:"allowedAudience"`
	Roles           []string `json:"roles"`
}

type AsgardeoApplicationOIDCResponse struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

func (a *Client) GetApplicationInfo(ctx context.Context, applicationId string) (*idp.ApplicationInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/server/v1/applications/%s", a.BaseURL, applicationId), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	res, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	defer res.Body.Close()

	var appInfo AsgardeoApplicationInfo

	err = json.NewDecoder(res.Body).Decode(&appInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	info := idp.ApplicationInfo{
		Id:          appInfo.Id,
		Name:        appInfo.Name,
		Description: appInfo.Description,
		ClientId:    appInfo.ClientId,
	}

	return &info, nil
}

func (a *Client) GetApplicationOIDC(ctx context.Context, applicationId string) (*idp.ApplicationOIDCInfo, error) {
	url := fmt.Sprintf("%s/api/server/v1/applications/%s/inbound-protocols/oidc", a.BaseURL, applicationId)
	// perform GET request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	res, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: %s", res.Status)
	}
	var oidcResponse AsgardeoApplicationOIDCResponse

	err = json.NewDecoder(res.Body).Decode(&oidcResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	oidcInfo := idp.ApplicationOIDCInfo{
		ClientId:     oidcResponse.ClientId,
		ClientSecret: oidcResponse.ClientSecret,
	}

	return &oidcInfo, nil
}

func (a *Client) CreateApplication(ctx context.Context, app *idp.Application) (*string, error) {
	// Prepare Asgardeo Application payload
	appInstance := AsgardeoApplication{
		Name:        app.Name,
		Description: app.Description,
		TemplateId:  ApplicationTemplateIDM2M,
		AssociatedRoles: AssociatedRole{
			AllowedAudience: "APPLICATION",
			Roles:           []string{},
		},
		InboundProtocolConfiguration: map[string]interface{}{
			"oidc": map[string]interface{}{
				"grantTypes": []string{"client_credentials"},
			},
		},
	}

	payload, err := json.Marshal(appInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal app instance: %w", err)
	}

	url := fmt.Sprintf("%s/api/server/v1/applications/", a.BaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with status %s", resp.Status)
	}

	location := resp.Header.Get("Location")
	applicationId := strings.TrimPrefix(location, url)

	return &applicationId, nil
}

func (a *Client) DeleteApplication(ctx context.Context, applicationId string) error {
	url := fmt.Sprintf("%s/api/server/v1/applications/%s", a.BaseURL, applicationId)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	res, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("request failed with status %s", res.Status)
	}

	return nil
}
