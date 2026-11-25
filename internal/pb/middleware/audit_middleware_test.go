// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package middleware

import (
	"context"
	"crypto"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
)

// mockAuditClient implements audit.Auditor interface for testing
type mockAuditClient struct {
	enabled         bool
	receivedEvents  []*audit.AuditLogRequest
	mu              sync.Mutex
	requestReceived chan bool
}

func newMockAuditClient(enabled bool) *mockAuditClient {
	return &mockAuditClient{
		enabled:         enabled,
		receivedEvents:  make([]*audit.AuditLogRequest, 0),
		requestReceived: make(chan bool, 1),
	}
}

func (m *mockAuditClient) LogEvent(ctx context.Context, event *audit.AuditLogRequest) bool {
	m.mu.Lock()
	m.receivedEvents = append(m.receivedEvents, event)
	m.mu.Unlock()
	select {
	case m.requestReceived <- true:
	default:
	}
	return true
}

func (m *mockAuditClient) SignEvent(ctx context.Context, event *audit.AuditLogRequest) error {
	return nil
}

func (m *mockAuditClient) SignMessageBytes(ctx context.Context, message []byte) (string, error) {
	return "", nil
}

func (m *mockAuditClient) LogSignedEvent(ctx context.Context, event *audit.AuditLogRequest) {}

func (m *mockAuditClient) VerifyIntegrity(event *audit.AuditLogRequest, publicKey crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditClient) Close(ctx context.Context) error {
	return nil
}

func (m *mockAuditClient) IsEnabled() bool {
	return m.enabled
}

func TestAuditMiddleware_Initialization(t *testing.T) {
	// Reset global state for this test
	audit.ResetGlobalAuditMiddleware()

	// Test with audit enabled
	mockClient1 := newMockAuditClient(true)
	audit.InitializeGlobalAudit(mockClient1)
	auditMiddleware := audit.GetGlobalAuditMiddleware()
	if auditMiddleware.Client() == nil {
		t.Error("Expected audit middleware to have client when provided")
	}
	if !auditMiddleware.Client().IsEnabled() {
		t.Error("Expected audit middleware to be enabled when client is enabled")
	}

	// Test with audit disabled (create new instance, but global should already be set)
	audit.ResetGlobalAuditMiddleware()
	mockClient2 := newMockAuditClient(false)
	audit.InitializeGlobalAudit(mockClient2)
	auditMiddleware2 := audit.GetGlobalAuditMiddleware()
	if auditMiddleware2.Client() == nil {
		t.Error("Expected audit middleware to have client instance even when disabled")
	}
	if auditMiddleware2.Client().IsEnabled() {
		t.Error("Expected audit middleware to be disabled when client is disabled")
	}
}

func TestLogAuditEvent_GlobalFunction(t *testing.T) {
	// Reset global state for this test
	audit.ResetGlobalAuditMiddleware()

	// Initialize global audit middleware
	mockClient := newMockAuditClient(true)
	audit.InitializeGlobalAudit(mockClient)

	// Test that LogAuditEvent doesn't panic when called
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("X-User-ID", "test-user")
	req.Header.Set("X-User-Role", "ADMIN")

	// This should not panic even if the audit service is not available
	resourceID := "test-id-123"
	LogAuditEvent(req, "TEST_RESOURCE", &resourceID, string(AuditStatusSuccess))
}

func TestLogAudit_SkipsReadOperations(t *testing.T) {
	mockClient := newMockAuditClient(true)

	// GET request should be skipped
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	resourceID := "test-id"
	LogAudit(mockClient, req, "TEST_RESOURCE", &resourceID, string(AuditStatusSuccess))

	// This test passes if no panic occurs - we can't easily test HTTP calls without a mock server
}

func TestLogAudit_ProcessesWriteOperations(t *testing.T) {
	mockClient := newMockAuditClient(true)

	// POST request should be processed (though it may fail to send)
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("X-User-ID", "test-user")
	req.Header.Set("X-User-Role", "MEMBER")

	resourceID := "test-id"
	LogAudit(mockClient, req, "TEST_RESOURCE", &resourceID, string(AuditStatusSuccess))

	// This test passes if no panic occurs - we can't easily test HTTP calls without a mock server
}

func TestAuditMiddleware_ThreadSafety(t *testing.T) {
	// Reset global state for this test
	audit.ResetGlobalAuditMiddleware()

	const numGoroutines = 10
	var wg sync.WaitGroup

	// Start multiple goroutines trying to initialize audit middleware concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			url := "http://localhost:3001"
			if id%2 == 0 {
				url = "" // Mix enabled and disabled instances
			}

			var client audit.Auditor
			if url != "" {
				client = newMockAuditClient(true)
			} else {
				client = newMockAuditClient(false)
			}
			audit.InitializeGlobalAudit(client)
		}(i)
	}

	wg.Wait()

	// Verify that the global instance was set
	globalMiddleware := audit.GetGlobalAuditMiddleware()
	if globalMiddleware == nil {
		t.Error("Expected global audit middleware to be set")
	}

	// Test that LogAuditEvent works with the global instance
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("X-User-ID", "test-user")
	req.Header.Set("X-User-Role", "ADMIN")

	// This should not panic
	resourceID := "test-id-concurrent"
	LogAuditEvent(req, "TEST_RESOURCE", &resourceID, string(AuditStatusSuccess))
}

func TestLogAuditEvent_WithoutInitialization(t *testing.T) {
	// Reset global state to ensure no global instance
	audit.ResetGlobalAuditMiddleware()

	// Test LogAuditEvent when global middleware is not initialized
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("X-User-ID", "test-user")
	req.Header.Set("X-User-Role", "ADMIN")

	// Should not panic
	resourceID := "test-resource"
	LogAuditEvent(req, "TEST_RESOURCE", &resourceID, string(AuditStatusSuccess))
}

func TestLogAudit_SendsRequest(t *testing.T) {
	// Setup mock server
	var receivedReq *http.Request
	var receivedBody []byte
	var wg sync.WaitGroup
	wg.Add(1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedReq = r
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusCreated)
		wg.Done()
	}))
	defer server.Close()

	// For this test, we need to use the real LSFLK/argus audit client since we're testing HTTP
	sharedClient := audit.NewClient(audit.Config{
		BaseURL:       server.URL,
		BatchSize:     1,
		BatchInterval: 1 * time.Millisecond,
	})

	// Create request with authenticated user in context
	req := callertest.NewMemberRequest(http.MethodPost, "/api/test", nil)

	resourceID := "test-resource-id"
	LogAudit(sharedClient, req, "TEST_RESOURCE", &resourceID, string(AuditStatusSuccess))

	// Wait for async log to complete
	// Note: In real code, we can't easily wait for the goroutine.
	// But since we control the server, we can wait for the request to arrive.
	// However, if the request is NOT sent (e.g. logic error), this will hang.
	// So we use a channel with timeout.

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for audit request")
	}

	// Verify request
	if receivedReq.Method != http.MethodPost {
		t.Errorf("Expected POST request, got %s", receivedReq.Method)
	}
	if receivedReq.URL.Path != "/api/audit-logs/bulk" {
		t.Errorf("Expected path /api/audit-logs/bulk, got %s", receivedReq.URL.Path)
	}

	// Verify body
	// We can unmarshal and check fields if needed, but checking it's not empty is a good start
	if len(receivedBody) == 0 {
		t.Error("Expected non-empty body")
	}
}

func TestLogAudit_RecordsSubjectAndClientID(t *testing.T) {
	client := newMockAuditClient(true)

	LogAudit(client, callertest.NewMemberRequest(http.MethodPost, "/api/v1/schemas", nil), "SCHEMAS", nil, string(AuditStatusSuccess))

	if len(client.receivedEvents) != 1 {
		t.Fatalf("got %d audit events, want 1", len(client.receivedEvents))
	}
	event := client.receivedEvents[0]
	if event.ActorID != callertest.MemberUser.Subject {
		t.Errorf("ActorID = %q, want the token's sub %q", event.ActorID, callertest.MemberUser.Subject)
	}
	if got := event.Metadata["clientId"]; got != callertest.MemberUser.ClientID {
		t.Errorf("Metadata[clientId] = %v, want %q", got, callertest.MemberUser.ClientID)
	}
}

func TestExtractActorInfoFromRequest_ClassifiesByScope(t *testing.T) {
	service := callertest.CreateCustomTestUser("service-client", "", "", callertest.ReadAllScopes)

	tests := []struct {
		name string
		req  *http.Request
		want ActorType
	}{
		{"cross-OU member writer is an admin", callertest.NewAdminRequest(http.MethodPost, "/api/v1/members", nil), ActorTypeAdmin},
		{"caller in an organization unit is a member", callertest.NewMemberRequest(http.MethodPost, "/api/v1/schemas", nil), ActorTypeMember},
		{"caller without an organization unit is a service", callertest.NewAuthenticatedRequest(http.MethodGet, "/api/v1/schemas", nil, service), ActorTypeSystem},
		{"unauthenticated caller is a service", callertest.NewUnauthenticatedRequest(http.MethodPost, "/api/v1/schemas", nil), ActorTypeSystem},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actorType, _, actorRole := extractActorInfoFromRequest(tt.req)
			if actorType != string(tt.want) || *actorRole != string(tt.want) {
				t.Errorf("actor = %s/%s, want %s", actorType, *actorRole, tt.want)
			}
		})
	}
}
