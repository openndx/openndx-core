package provider

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/shared/monitoring"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/logger"
	"github.com/ginaxu1/gov-dx-sandbox/exchange/orchestration-engine/pkg/auth"
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
func (p *Provider) PerformRequest(ctx context.Context, reqBody []byte) (resp *http.Response, err error) {
	// 1. Create Request
	req, err := http.NewRequestWithContext(ctx, "POST", p.ServiceUrl, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	defer func() {
		monitoring.RecordExternalCall(p.ServiceKey, "provider_request", time.Since(start), err)
	}()

	if p.Auth != nil {
		switch p.Auth.Type {
		case auth.AuthTypeOAuth2:
			if p.OAuth2Config == nil {
				err = fmt.Errorf("OAuth2Config is nil")
				logger.Log.Error(err.Error(), "providerKey", p.ServiceKey)
				return
			}

			client := p.OAuth2Config.Client(ctx)
			resp, err = client.Do(req)
			return
		case auth.AuthTypeAPIKey:
			req.Header.Set(p.Auth.APIKeyName, p.Auth.APIKeyValue)
		}
	}

	// Default client execution (for API Key or no auth)
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err = client.Do(req)
	return
}
