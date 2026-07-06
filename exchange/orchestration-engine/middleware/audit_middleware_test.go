package middleware

import (
	"context"
	"crypto"
	"sync"
	"testing"
	"time"

	auditpkg "github.com/LSFLK/argus/pkg/audit"
)

// mockAuditClient implements auditpkg.AuditClient interface for testing
type mockAuditClient struct {
	enabled         bool
	receivedEvents  []*auditpkg.AuditLogRequest
	mu              sync.Mutex
	requestReceived chan bool
}

func newMockAuditClient(enabled bool) *mockAuditClient {
	return &mockAuditClient{
		enabled:         enabled,
		receivedEvents:  make([]*auditpkg.AuditLogRequest, 0),
		requestReceived: make(chan bool, 1),
	}
}

func (m *mockAuditClient) LogEvent(ctx context.Context, event *auditpkg.AuditLogRequest) bool {
	m.mu.Lock()
	m.receivedEvents = append(m.receivedEvents, event)
	m.mu.Unlock()
	select {
	case m.requestReceived <- true:
	default:
	}
	return true
}

func (m *mockAuditClient) SignEvent(ctx context.Context, event *auditpkg.AuditLogRequest) error {
	return nil
}

func (m *mockAuditClient) SignMessageBytes(ctx context.Context, message []byte) (string, error) {
	return "", nil
}

func (m *mockAuditClient) LogSignedEvent(ctx context.Context, event *auditpkg.AuditLogRequest) {}

func (m *mockAuditClient) VerifyIntegrity(event *auditpkg.AuditLogRequest, publicKey crypto.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockAuditClient) Close(ctx context.Context) error {
	return nil
}

func (m *mockAuditClient) IsEnabled() bool {
	return m.enabled
}

func TestLogAuditEvent(t *testing.T) {
	// Reset global middleware before test
	auditpkg.ResetGlobalAuditMiddleware()

	// Create a mock audit client
	mockClient := newMockAuditClient(true)

	// Initialize audit middleware with mock client
	auditpkg.InitializeGlobalAudit(mockClient)

	// Create test audit request using shared audit DTO
	traceID := "550e8400-e29b-41d4-a716-446655440000"
	eventType := "POLICY_CHECK"
	testRequest := &auditpkg.AuditLogRequest{
		TraceID:    &traceID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		EventType:  eventType,
		Status:     auditpkg.StatusSuccess,
		ActorType:  "SERVICE",
		ActorID:    "orchestration-engine",
		TargetType: "SERVICE",
		Metadata: map[string]interface{}{
			"appId": "test-app-123",
		},
	}

	// Call LogAuditEvent from shared package
	auditpkg.LogAuditEvent(context.Background(), testRequest)

	// Wait for the async operation to complete with timeout
	select {
	case <-mockClient.requestReceived:
		// Request received successfully
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for audit request")
	}

	// Verify the request was received
	mockClient.mu.Lock()
	defer mockClient.mu.Unlock()

	if len(mockClient.receivedEvents) == 0 {
		t.Fatal("No request received by mock client")
	}

	receivedRequest := mockClient.receivedEvents[0]
	expectedActorID := "orchestration-engine"
	if receivedRequest.ActorID != expectedActorID {
		t.Errorf("Expected ActorID %s, got %s", expectedActorID, receivedRequest.ActorID)
	}

	expectedStatus := auditpkg.StatusSuccess
	if receivedRequest.Status != expectedStatus {
		t.Errorf("Expected Status %s, got %s", expectedStatus, receivedRequest.Status)
	}
}

func TestLogAuditEventWhenNotConfigured(t *testing.T) {
	// Reset global middleware before test
	auditpkg.ResetGlobalAuditMiddleware()

	// Initialize audit middleware with disabled mock client
	mockClient := newMockAuditClient(false)
	auditpkg.InitializeGlobalAudit(mockClient)

	// Create test audit request using shared audit DTO
	traceID := "550e8400-e29b-41d4-a716-446655440000"
	eventType := "POLICY_CHECK"
	testRequest := &auditpkg.AuditLogRequest{
		TraceID:    &traceID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		EventType:  eventType,
		Status:     auditpkg.StatusSuccess,
		ActorType:  "SERVICE",
		ActorID:    "orchestration-engine",
		TargetType: "SERVICE",
	}

	// This should not panic or cause errors
	auditpkg.LogAuditEvent(context.Background(), testRequest)

	// Give some time for any potential async operation
	time.Sleep(50 * time.Millisecond)

	// Test passes if no panic occurs
}

func TestLogAuditEventWhenGlobalMiddlewareNotInitialized(t *testing.T) {
	// Reset global middleware to simulate uninitialized state
	auditpkg.ResetGlobalAuditMiddleware()

	// Create test audit request using shared audit DTO
	traceID := "550e8400-e29b-41d4-a716-446655440000"
	eventType := "POLICY_CHECK"
	testRequest := &auditpkg.AuditLogRequest{
		TraceID:    &traceID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		EventType:  eventType,
		Status:     auditpkg.StatusSuccess,
		ActorType:  "SERVICE",
		ActorID:    "orchestration-engine",
		TargetType: "SERVICE",
	}

	// This should not panic or cause errors
	auditpkg.LogAuditEvent(context.Background(), testRequest)

	// Test passes if no panic occurs
}
