package provider

import (
	"testing"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/auth"
)

func TestNewProviderHandler(t *testing.T) {
	tests := []struct {
		name             string
		providers        []*Provider
		expectedCount    int
		expectHttpClient bool
		expectedTimeout  time.Duration
	}{
		{
			name: "creates handler with valid providers",
			providers: []*Provider{
				NewProvider("provider1", "http://example.com", "schema1", nil),
				NewProvider("provider2", "http://example2.com", "schema2", nil),
			},
			expectedCount:    2,
			expectHttpClient: true,
			expectedTimeout:  10 * time.Second,
		},
		{
			name: "filters out nil providers",
			providers: []*Provider{
				NewProvider("provider1", "http://example.com", "schema1", nil),
				nil,
				NewProvider("provider2", "http://example2.com", "schema2", nil),
			},
			expectedCount:    2,
			expectHttpClient: true,
			expectedTimeout:  10 * time.Second,
		},
		{
			name: "filters out providers with empty service key",
			providers: []*Provider{
				NewProvider("provider1", "http://example.com", "schema1", nil),
				NewProvider("", "http://example2.com", "schema2", nil),
				NewProvider("provider3", "http://example3.com", "schema3", nil),
			},
			expectedCount:    2,
			expectHttpClient: true,
			expectedTimeout:  10 * time.Second,
		},
		{
			name:             "creates handler with empty providers list",
			providers:        []*Provider{},
			expectedCount:    0,
			expectHttpClient: true,
			expectedTimeout:  10 * time.Second,
		},
		{
			name:             "creates handler with nil providers list",
			providers:        nil,
			expectedCount:    0,
			expectHttpClient: true,
			expectedTimeout:  10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewProviderHandler(tt.providers)

			if handler == nil {
				t.Fatal("NewProviderHandler returned nil")
			}

			if len(handler.Providers) != tt.expectedCount {
				t.Errorf("Expected %d providers, got %d", tt.expectedCount, len(handler.Providers))
			}

			if tt.expectHttpClient && handler.HttpClient == nil {
				t.Error("Expected HttpClient to be set, got nil")
			}

			if handler.HttpClient != nil {
				if handler.HttpClient.Timeout != tt.expectedTimeout {
					t.Errorf("Expected timeout %v, got %v", tt.expectedTimeout, handler.HttpClient.Timeout)
				}
			}

			// Verify that each valid provider has the client set
			for _, p := range handler.Providers {
				if p.Client == nil {
					t.Errorf("Provider %s has nil client", p.ServiceKey)
				}
				if p.Client != handler.HttpClient {
					t.Errorf("Provider %s client doesn't match handler's HttpClient", p.ServiceKey)
				}
			}
		})
	}
}

func TestHandler_GetProvider(t *testing.T) {
	providers := []*Provider{
		NewProvider("provider1", "http://example1.com", "schema1", nil),
		NewProvider("provider2", "http://example2.com", "schema2", nil),
		NewProvider("provider3", "http://example3.com", "schema1", nil),
	}
	handler := NewProviderHandler(providers)

	tests := []struct {
		name         string
		serviceKey   string
		schemaID     string
		expectExists bool
		expectURL    string
	}{
		{
			name:         "finds existing provider by service key and schema ID",
			serviceKey:   "provider1",
			schemaID:     "schema1",
			expectExists: true,
			expectURL:    "http://example1.com",
		},
		{
			name:         "finds second provider",
			serviceKey:   "provider2",
			schemaID:     "schema2",
			expectExists: true,
			expectURL:    "http://example2.com",
		},
		{
			name:         "finds provider with matching service key and schema ID combination",
			serviceKey:   "provider3",
			schemaID:     "schema1",
			expectExists: true,
			expectURL:    "http://example3.com",
		},
		{
			name:         "returns false for non-existent service key",
			serviceKey:   "nonexistent",
			schemaID:     "schema1",
			expectExists: false,
		},
		{
			name:         "returns false for non-existent schema ID",
			serviceKey:   "provider1",
			schemaID:     "nonexistent",
			expectExists: false,
		},
		{
			name:         "returns false for empty service key",
			serviceKey:   "",
			schemaID:     "schema1",
			expectExists: false,
		},
		{
			name:         "returns false for empty schema ID",
			serviceKey:   "provider1",
			schemaID:     "",
			expectExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, exists := handler.GetProvider(tt.serviceKey, tt.schemaID)

			if exists != tt.expectExists {
				t.Errorf("Expected exists=%v, got %v", tt.expectExists, exists)
			}

			if tt.expectExists {
				if provider == nil {
					t.Fatal("Expected provider to be non-nil when exists is true")
				}
				if provider.ServiceKey != tt.serviceKey {
					t.Errorf("Expected service key %s, got %s", tt.serviceKey, provider.ServiceKey)
				}
				if provider.SchemaID != tt.schemaID {
					t.Errorf("Expected schema ID %s, got %s", tt.schemaID, provider.SchemaID)
				}
				if provider.ServiceUrl != tt.expectURL {
					t.Errorf("Expected URL %s, got %s", tt.expectURL, provider.ServiceUrl)
				}
			} else {
				if provider != nil {
					t.Error("Expected provider to be nil when exists is false")
				}
			}
		})
	}
}

func TestHandler_AddProvider(t *testing.T) {
	tests := []struct {
		name             string
		initialProviders []*Provider
		providerToAdd    *Provider
		expectedCount    int
	}{
		{
			name: "adds provider to existing handler",
			initialProviders: []*Provider{
				NewProvider("provider1", "http://example1.com", "schema1", nil),
			},
			providerToAdd: NewProvider("provider2", "http://example2.com", "schema2", nil),
			expectedCount: 2,
		},
		{
			name:             "adds provider to empty handler",
			initialProviders: []*Provider{},
			providerToAdd:    NewProvider("provider1", "http://example1.com", "schema1", nil),
			expectedCount:    1,
		},
		{
			name: "adds provider with auth config",
			initialProviders: []*Provider{
				NewProvider("provider1", "http://example1.com", "schema1", nil),
			},
			providerToAdd: NewProvider("provider2", "http://example2.com", "schema2", &auth.AuthConfig{
				Type:        auth.AuthTypeAPIKey,
				APIKeyName:  "X-API-Key",
				APIKeyValue: "test-key",
			}),
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewProviderHandler(tt.initialProviders)
			initialCount := len(handler.Providers)

			handler.AddProvider(tt.providerToAdd)

			if len(handler.Providers) != tt.expectedCount {
				t.Errorf("Expected %d providers, got %d", tt.expectedCount, len(handler.Providers))
			}

			if len(handler.Providers) != initialCount+1 {
				t.Errorf("Expected provider count to increase by 1, was %d, now %d", initialCount, len(handler.Providers))
			}

			// Verify the added provider has the client set
			addedProvider := handler.Providers[len(handler.Providers)-1]
			if addedProvider.Client != handler.HttpClient {
				t.Error("Added provider's client doesn't match handler's HttpClient")
			}

			// Verify the added provider is the one we added
			if addedProvider.ServiceKey != tt.providerToAdd.ServiceKey {
				t.Errorf("Expected service key %s, got %s", tt.providerToAdd.ServiceKey, addedProvider.ServiceKey)
			}
		})
	}
}

func TestHandler_ConcurrentAccess(t *testing.T) {
	// Test concurrent reads and writes to ensure proper mutex usage
	handler := NewProviderHandler([]*Provider{
		NewProvider("provider1", "http://example1.com", "schema1", nil),
	})

	done := make(chan bool)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = handler.GetProvider("provider1", "schema1")
			}
			done <- true
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(idx int) {
			for j := 0; j < 50; j++ {
				p := NewProvider("test", "http://test.com", "test", nil)
				handler.AddProvider(p)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 15; i++ {
		<-done
	}

	// Just verify the handler still works
	_, exists := handler.GetProvider("provider1", "schema1")
	if !exists {
		t.Error("Original provider should still exist after concurrent operations")
	}
}
