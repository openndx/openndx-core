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
		Client:      oauthConfig.Client(context.Background()),
	}
}
