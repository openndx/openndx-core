package provider

import (
	"net/http"
	"sync"
	"time"
)

// Handler is the main struct that holds all the provider handling information
type Handler struct {
	mu         sync.RWMutex
	Providers  []*Provider
	HttpClient *http.Client
}

// NewProviderHandler creates a new ProviderHandler with the given providers.
func NewProviderHandler(providers []*Provider) *Handler {
	providerMap := make([]*Provider, 0)

	// Create an http client with a 10 seconds timeout
	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: http.DefaultTransport,
	}

	for _, p := range providers {
		if p != nil && p.ServiceKey != "" {
			providerMap = append(providerMap, p)
			p.Client = httpClient
		}
	}

	return &Handler{
		Providers:  providerMap,
		HttpClient: httpClient,
	}
}

// GetProvider retrieves a provider by its service key.
func (h *Handler) GetProvider(serviceKey, schemaId string) (*Provider, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	// Find provider by service key and schema ID
	var p *Provider
	exists := false
	for _, provider := range h.Providers {
		if provider == nil {
			continue
		}
		if provider.ServiceKey == serviceKey && provider.SchemaID == schemaId {
			p = provider
			exists = true
			break
		}
	}
	return p, exists
}

// AddProvider adds a new provider to the handler.
func (h *Handler) AddProvider(provider *Provider) {
	if provider == nil || provider.ServiceKey == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.Providers = append(h.Providers, provider)
	provider.Client = h.HttpClient
}
