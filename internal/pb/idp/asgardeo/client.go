package asgardeo

import (
	"context"
	"net/http"

	"golang.org/x/oauth2/clientcredentials"
)

type Client struct {
	BaseURL     string
	OAuthConfig *clientcredentials.Config
	Client      *http.Client
}

func NewClient(baseUrl string, clientId string, clientSecret string, scopes []string) *Client {
	oauthConfig := &clientcredentials.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		TokenURL:     baseUrl + "/oauth2/token",
		Scopes:       scopes,
	}

	return &Client{
		BaseURL:     baseUrl,
		OAuthConfig: oauthConfig,
		// Kept for callers/tests that inspect Client; request methods use httpClient(ctx)
		// so token acquisition respects the caller's cancellation and deadlines.
		Client: oauthConfig.Client(context.Background()),
	}
}

// httpClient returns an OAuth2 HTTP client whose token fetch uses ctx.
func (a *Client) httpClient(ctx context.Context) *http.Client {
	if ctx == nil {
		ctx = context.Background()
	}
	return a.OAuthConfig.Client(ctx)
}
