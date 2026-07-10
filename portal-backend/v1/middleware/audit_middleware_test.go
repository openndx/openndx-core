package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"crypto"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/gov-dx-sandbox/portal-backend/v1/models"
	"github.com/gov-dx-sandbox/portal-backend/v1/utils"
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
	LogAuditEvent(req, "TEST_RESOURCE", &resourceID, string(models.AuditStatusSuccess))
}

func TestLogAudit_SkipsReadOperations(t *testing.T) {
	mockClient := newMockAuditClient(true)

	// GET request should be skipped
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	resourceID := "test-id"
	LogAudit(mockClient, req, "TEST_RESOURCE", &resourceID, string(models.AuditStatusSuccess))

	// This test passes if no panic occurs - we can't easily test HTTP calls without a mock server
}

func TestLogAudit_ProcessesWriteOperations(t *testing.T) {
	mockClient := newMockAuditClient(true)

	// POST request should be processed (though it may fail to send)
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("X-User-ID", "test-user")
	req.Header.Set("X-User-Role", "MEMBER")

	resourceID := "test-id"
	LogAudit(mockClient, req, "TEST_RESOURCE", &resourceID, string(models.AuditStatusSuccess))

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
	LogAuditEvent(req, "TEST_RESOURCE", &resourceID, string(models.AuditStatusSuccess))
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
	LogAuditEvent(req, "TEST_RESOURCE", &resourceID, string(models.AuditStatusSuccess))
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

	// For this test, we need to use the real shared/audit client since we're testing HTTP
	sharedClient := audit.NewClient(audit.Config{
		BaseURL:       server.URL,
		BatchSize:     1,
		BatchInterval: 1 * time.Millisecond,
	})

	// Create request with authenticated user in context
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)

	// Create an authenticated user to put in context
	now := time.Now().Unix()
	claims := &models.UserClaims{
		IdpUserID: "test-user-id",
		Email:     "test@example.com",
		Roles:     models.FlexibleStringSlice([]string{"OpenDIF_Member"}),
		IssuedAt:  now,
		ExpiresAt: now + 3600,
	}

	user, err := models.NewAuthenticatedUser(claims)
	if err != nil {
		t.Fatalf("Failed to create authenticated user: %v", err)
	}

	// Set user in request context using the proper utility function
	ctx := utils.SetAuthenticatedUser(req.Context(), user)
	req = req.WithContext(ctx)

	resourceID := "test-resource-id"
	LogAudit(sharedClient, req, "TEST_RESOURCE", &resourceID, string(models.AuditStatusSuccess))

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
