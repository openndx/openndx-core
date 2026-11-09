package middleware

import (
	"context"
	"crypto"
	"errors"
	"reflect"
	"sync"
	"testing"

	auditpkg "github.com/LSFLK/argus/pkg/audit"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
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

// onlyEvent returns the single event the client received.
func (m *mockAuditClient) onlyEvent(t *testing.T) *auditpkg.AuditLogRequest {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(m.receivedEvents))
	}
	return m.receivedEvents[0]
}

// responseMetadata returns the event's response metadata.
func responseMetadata(t *testing.T, event *auditpkg.AuditLogRequest) map[string]interface{} {
	t.Helper()
	meta, ok := event.Metadata["responseMetadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected response metadata, got %v", event.Metadata)
	}
	return meta
}

func TestAuditor_RequestReceived(t *testing.T) {
	mockClient := newMockAuditClient(true)
	auditor := NewAuditor(mockClient, "SERVICE", "orchestration-engine")

	ctx := auditor.RequestReceived(context.Background(), "app-1", "query { a }")

	event := mockClient.onlyEvent(t)
	if event.EventType != "DATA_REQUEST" || event.ActorType != "APPLICATION" || event.ActorID != "app-1" {
		t.Errorf("Expected DATA_REQUEST by APPLICATION/app-1, got %s by %s/%s", event.EventType, event.ActorType, event.ActorID)
	}
	if event.TraceID == nil || *event.TraceID == "" {
		t.Error("Expected a trace ID on the event")
	}
	if ctx == context.Background() {
		t.Error("Expected the returned context to carry the generated trace ID")
	}
}

func TestAuditor_PolicyCheck(t *testing.T) {
	t.Run("denied", func(t *testing.T) {
		mockClient := newMockAuditClient(true)
		auditor := NewAuditor(mockClient, "SERVICE", "orchestration-engine")

		auditor.PolicyCheck(context.Background(), "app-1", &policy.PdpRequest{AppId: "app-1"},
			&policy.PdpResponse{AppAuthorized: false, UnauthorizedFields: []policy.ConsentRequiredField{{FieldName: "person.name"}}}, nil)

		event := mockClient.onlyEvent(t)
		if event.EventType != "POLICY_CHECK" || event.Status != auditpkg.StatusFailure {
			t.Errorf("Expected a failed POLICY_CHECK, got %s %s", event.Status, event.EventType)
		}
		if event.ActorType != "SERVICE" || event.ActorID != "orchestration-engine" {
			t.Errorf("Expected configured actor SERVICE/orchestration-engine, got %s/%s", event.ActorType, event.ActorID)
		}
		if event.TargetID == nil || *event.TargetID != "policy-decision-point" {
			t.Errorf("Expected target policy-decision-point, got %v", event.TargetID)
		}
		if _, ok := responseMetadata(t, event)["unauthorizedFields"]; !ok {
			t.Error("Expected unauthorized fields in the response metadata")
		}
	})

	t.Run("records per-field outcomes and obligations", func(t *testing.T) {
		mockClient := newMockAuditClient(true)
		auditor := NewAuditor(mockClient, "SERVICE", "orchestration-engine")

		fields := []policy.FieldOutcome{
			{SchemaID: "drp", FieldName: "person.name", Outcome: policy.OutcomeAllow, Obligations: []string{"audit.consent_bypassed"}},
			{SchemaID: "drp", FieldName: "person.photo", Outcome: policy.OutcomeConsentRequired},
		}
		auditor.PolicyCheck(context.Background(), "app-1", &policy.PdpRequest{AppId: "app-1"},
			&policy.PdpResponse{
				Fields:                  fields,
				AppAuthorized:           true,
				AppRequiresOwnerConsent: true,
				ConsentRequiredFields:   []policy.ConsentRequiredField{{SchemaID: "drp", FieldName: "person.photo"}},
			}, nil)

		event := mockClient.onlyEvent(t)
		if event.Status != auditpkg.StatusSuccess {
			t.Errorf("Expected a successful POLICY_CHECK, got %s", event.Status)
		}
		meta := responseMetadata(t, event)
		got, ok := meta["fields"].([]policy.FieldOutcome)
		if !ok || !reflect.DeepEqual(got, fields) {
			t.Errorf("Expected fields %v in the response metadata, got %v", fields, meta["fields"])
		}
		if meta["consentRequired"] != true {
			t.Errorf("Expected consentRequired alongside fields, got %v", meta["consentRequired"])
		}
	})

	t.Run("omits fields when the PDP returns none", func(t *testing.T) {
		mockClient := newMockAuditClient(true)
		auditor := NewAuditor(mockClient, "SERVICE", "orchestration-engine")

		auditor.PolicyCheck(context.Background(), "app-1", &policy.PdpRequest{AppId: "app-1"},
			&policy.PdpResponse{AppAuthorized: true}, nil)

		if _, ok := responseMetadata(t, mockClient.onlyEvent(t))["fields"]; ok {
			t.Error("Expected no fields in the response metadata")
		}
	})

	t.Run("request failed", func(t *testing.T) {
		mockClient := newMockAuditClient(true)
		auditor := NewAuditor(mockClient, "SERVICE", "orchestration-engine")

		auditor.PolicyCheck(context.Background(), "app-1", nil, nil, errors.New("pdp unavailable"))

		event := mockClient.onlyEvent(t)
		if event.Status != auditpkg.StatusFailure || responseMetadata(t, event)["error"] != "pdp unavailable" {
			t.Errorf("Expected a failure with the PDP error, got %s %v", event.Status, event.Metadata)
		}
	})
}

func TestAuditor_ConsentCheck(t *testing.T) {
	mockClient := newMockAuditClient(true)
	auditor := NewAuditor(mockClient, "SERVICE", "orchestration-engine")

	auditor.ConsentCheck(context.Background(), "app-1", "owner-1", &consent.CreateConsentRequest{},
		&consent.ConsentResponseInternalView{ConsentID: "consent-1", Status: consent.StatusApproved}, nil)

	event := mockClient.onlyEvent(t)
	if event.EventType != "CONSENT_CHECK" || event.Status != auditpkg.StatusSuccess {
		t.Errorf("Expected a successful CONSENT_CHECK, got %s %s", event.Status, event.EventType)
	}
	if responseMetadata(t, event)["consentId"] != "consent-1" {
		t.Errorf("Expected consent-1 in the response metadata, got %v", event.Metadata)
	}
}

func TestAuditor_ProviderFetch(t *testing.T) {
	fetch := federator.ProviderFetch{
		ApplicationID:   "app-1",
		SchemaID:        "drp-schema",
		Query:           "query { person { name } }",
		RequestedFields: []string{"person.name"},
	}

	t.Run("success", func(t *testing.T) {
		mockClient := newMockAuditClient(true)
		auditor := NewAuditor(mockClient, "SERVICE", "orchestration-engine")

		auditor.ProviderFetch(context.Background(), fetch, &graphql.Response{Data: map[string]interface{}{"person": nil}}, nil)

		event := mockClient.onlyEvent(t)
		if event.EventType != "PROVIDER_FETCH" || event.Status != auditpkg.StatusSuccess {
			t.Errorf("Expected a successful PROVIDER_FETCH, got %s %s", event.Status, event.EventType)
		}
		if event.TargetID == nil || *event.TargetID != "drp-schema" {
			t.Errorf("Expected target drp-schema, got %v", event.TargetID)
		}
		meta := responseMetadata(t, event)
		if meta["applicationId"] != "app-1" || meta["schemaId"] != "drp-schema" {
			t.Errorf("Expected app-1 and drp-schema in the response metadata, got %v", meta)
		}
		if fields, _ := meta["requestedFields"].([]string); len(fields) != 1 || fields[0] != "person.name" {
			t.Errorf("Expected requested field person.name, got %v", meta["requestedFields"])
		}
	})

	t.Run("failure", func(t *testing.T) {
		mockClient := newMockAuditClient(true)
		auditor := NewAuditor(mockClient, "SERVICE", "orchestration-engine")

		auditor.ProviderFetch(context.Background(), fetch, nil, errors.New("connection refused"))

		event := mockClient.onlyEvent(t)
		if event.Status != auditpkg.StatusFailure || responseMetadata(t, event)["error"] != "connection refused" {
			t.Errorf("Expected a failure with the provider error, got %s %v", event.Status, event.Metadata)
		}
	})
}

func TestAuditor_NilClient(t *testing.T) {
	auditor := NewAuditor(nil, "SERVICE", "orchestration-engine")

	// This should not panic or cause errors
	ctx := context.Background()
	auditor.RequestReceived(ctx, "app-1", "query { a }")
	auditor.PolicyCheck(ctx, "app-1", nil, nil, nil)
	auditor.ConsentCheck(ctx, "app-1", "", nil, nil, nil)
	auditor.ProviderFetch(ctx, federator.ProviderFetch{}, nil, nil)
}
