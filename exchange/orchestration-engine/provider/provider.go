package provider

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/auth"
	"golang.org/x/oauth2/clientcredentials"
)

// Provider struct that represents a provider attributes.
type Provider struct {
	Client       *http.Client
	ServiceUrl   string           `json:"providerUrl,omitempty"`
	SchemaID     string           `json:"schemaId,omitempty"`
	ServiceKey   string           `json:"providerKey,omitempty"`
	Auth         *auth.AuthConfig `json:"auth,omitempty"`
	OAuth2Config *clientcredentials.Config
	Headers      map[string]string `json:"headers,omitempty"`
	tokenMu      sync.RWMutex
}

func NewProvider(serviceKey, serviceUrl, schemaID string, authConfig *auth.AuthConfig) *Provider {
	provider := &Provider{
		Client:     &http.Client{},
		ServiceUrl: serviceUrl,
		SchemaID:   schemaID,
		ServiceKey: serviceKey,
		Auth:       authConfig,
		Headers:    make(map[string]string),
	}

	if authConfig != nil && authConfig.Type == auth.AuthTypeOAuth2 {
		provider.OAuth2Config = &clientcredentials.Config{
			ClientID:     authConfig.ClientID,
			ClientSecret: authConfig.ClientSecret,
			TokenURL:     authConfig.TokenURL,
			Scopes:       authConfig.Scopes,
		}
	}

	return provider
}

// PerformRequest performs the HTTP request to the provider with necessary authentication.
func (p *Provider) PerformRequest(ctx context.Context, reqBody []byte) (*http.Response, error) {
	// 1. Create Request
	req, err := http.NewRequestWithContext(ctx, "POST", p.ServiceUrl, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	if p.Auth != nil {
		switch p.Auth.Type {
		case auth.AuthTypeOAuth2:
			if p.OAuth2Config == nil {
				logger.Log.Error("OAuth2Config is nil", "providerKey", p.ServiceKey)
				return nil, fmt.Errorf("OAuth2Config is nil")
			}

			client := p.OAuth2Config.Client(ctx)
			return client.Do(req) // Use context with request
		case auth.AuthTypeAPIKey:
			req.Header.Set(p.Auth.APIKeyName, p.Auth.APIKeyValue)
		}
	}

	// Default client execution (for API Key or no auth)
	return p.Client.Do(req)
}
