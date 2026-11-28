// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package middleware

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/openndx/openndx-core/internal/audit"
	"github.com/openndx/openndx-core/internal/oe/consent"
	"github.com/openndx/openndx-core/internal/oe/federator"
	"github.com/openndx/openndx-core/internal/oe/pkg/graphql"
	"github.com/openndx/openndx-core/internal/oe/policy"
)

// onlyRecord returns the single record sink received.
func onlyRecord(t *testing.T, sink *audit.CaptureSink) audit.Record {
	t.Helper()
	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
	return records[0]
}

// responseMetadata returns the record's response metadata.
func responseMetadata(t *testing.T, record audit.Record) map[string]interface{} {
	t.Helper()
	meta, ok := record.Metadata["responseMetadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected response metadata, got %v", record.Metadata)
	}
	return meta
}

func TestAuditor_RequestReceived(t *testing.T) {
	sink := &audit.CaptureSink{}
	auditor := NewAuditor(sink, "SERVICE", "orchestration-engine")

	ctx := auditor.RequestReceived(context.Background(), "app-1", `query { person(nic: "FL-2954817360") { name } }`)

	event := onlyRecord(t, sink)
	if event.EventType != "DATA_REQUEST" || event.ActorType != "APPLICATION" || event.ActorID != "app-1" {
		t.Errorf("Expected DATA_REQUEST by APPLICATION/app-1, got %s by %s/%s", event.EventType, event.ActorType, event.ActorID)
	}
	if event.TraceID == "" {
		t.Error("Expected a trace ID on the event")
	}
	if ctx == context.Background() {
		t.Error("Expected the returned context to carry the generated trace ID")
	}
	requestMetadata, _ := event.Metadata["requestMetadata"].(map[string]any)
	if requestMetadata["applicationId"] != "app-1" {
		t.Errorf("Expected app-1 in the request metadata, got %v", event.Metadata)
	}
	if _, ok := requestMetadata["query"]; ok {
		t.Error("The consumer query can identify a citizen and must not be recorded")
	}
}

func TestAuditor_PolicyCheck(t *testing.T) {
	t.Run("denied", func(t *testing.T) {
		sink := &audit.CaptureSink{}
		auditor := NewAuditor(sink, "SERVICE", "orchestration-engine")

		auditor.PolicyCheck(context.Background(), "app-1", &policy.PdpRequest{AppId: "app-1"},
			&policy.PdpResponse{AppAuthorized: false, UnauthorizedFields: []policy.ConsentRequiredField{{FieldName: "person.name"}}}, nil)

		event := onlyRecord(t, sink)
		if event.EventType != "POLICY_CHECK" || event.Status != audit.StatusFailure {
			t.Errorf("Expected a failed POLICY_CHECK, got %s %s", event.Status, event.EventType)
		}
		if event.ActorType != "SERVICE" || event.ActorID != "orchestration-engine" {
			t.Errorf("Expected configured actor SERVICE/orchestration-engine, got %s/%s", event.ActorType, event.ActorID)
		}
		if event.TargetID != "policy-decision-point" {
			t.Errorf("Expected target policy-decision-point, got %v", event.TargetID)
		}
		if _, ok := responseMetadata(t, event)["unauthorizedFields"]; !ok {
			t.Error("Expected unauthorized fields in the response metadata")
		}
	})

	t.Run("records per-field outcomes and obligations", func(t *testing.T) {
		sink := &audit.CaptureSink{}
		auditor := NewAuditor(sink, "SERVICE", "orchestration-engine")

		fields := []policy.FieldOutcome{
			{SchemaID: "identity-registry", FieldName: "person.name", Outcome: policy.OutcomeAllow, Obligations: []string{"audit.consent_bypassed"}},
			{SchemaID: "identity-registry", FieldName: "person.photo", Outcome: policy.OutcomeConsentRequired},
		}
		auditor.PolicyCheck(context.Background(), "app-1", &policy.PdpRequest{AppId: "app-1"},
			&policy.PdpResponse{
				Fields:                  fields,
				AppAuthorized:           true,
				AppRequiresOwnerConsent: true,
				ConsentRequiredFields:   []policy.ConsentRequiredField{{SchemaID: "identity-registry", FieldName: "person.photo"}},
			}, nil)

		event := onlyRecord(t, sink)
		if event.Status != audit.StatusSuccess {
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
		sink := &audit.CaptureSink{}
		auditor := NewAuditor(sink, "SERVICE", "orchestration-engine")

		auditor.PolicyCheck(context.Background(), "app-1", &policy.PdpRequest{AppId: "app-1"},
			&policy.PdpResponse{AppAuthorized: true}, nil)

		if _, ok := responseMetadata(t, onlyRecord(t, sink))["fields"]; ok {
			t.Error("Expected no fields in the response metadata")
		}
	})

	t.Run("request failed", func(t *testing.T) {
		sink := &audit.CaptureSink{}
		auditor := NewAuditor(sink, "SERVICE", "orchestration-engine")

		auditor.PolicyCheck(context.Background(), "app-1", nil, nil, errors.New("pdp unavailable"))

		event := onlyRecord(t, sink)
		if event.Status != audit.StatusFailure || responseMetadata(t, event)["error"] != "pdp unavailable" {
			t.Errorf("Expected a failure with the PDP error, got %s %v", event.Status, event.Metadata)
		}
	})
}

func TestAuditor_ConsentCheck(t *testing.T) {
	sink := &audit.CaptureSink{}
	auditor := NewAuditor(sink, "SERVICE", "orchestration-engine")

	auditor.ConsentCheck(context.Background(), "app-1", "owner-1", &consent.CreateConsentRequest{},
		&consent.ConsentResponseInternalView{ConsentID: "consent-1", Status: consent.StatusApproved}, nil)

	event := onlyRecord(t, sink)
	if event.EventType != "CONSENT_CHECK" || event.Status != audit.StatusSuccess {
		t.Errorf("Expected a successful CONSENT_CHECK, got %s %s", event.Status, event.EventType)
	}
	if responseMetadata(t, event)["consentId"] != "consent-1" {
		t.Errorf("Expected consent-1 in the response metadata, got %v", event.Metadata)
	}
}

func TestAuditor_ProviderFetch(t *testing.T) {
	fetch := federator.ProviderFetch{
		ApplicationID:   "app-1",
		SchemaID:        "identity-registry-schema",
		Query:           "query { person { name } }",
		RequestedFields: []string{"person.name"},
	}

	t.Run("success", func(t *testing.T) {
		sink := &audit.CaptureSink{}
		auditor := NewAuditor(sink, "SERVICE", "orchestration-engine")

		auditor.ProviderFetch(context.Background(), fetch, &graphql.Response{Data: map[string]interface{}{"person": nil}}, nil)

		event := onlyRecord(t, sink)
		if event.EventType != "PROVIDER_FETCH" || event.Status != audit.StatusSuccess {
			t.Errorf("Expected a successful PROVIDER_FETCH, got %s %s", event.Status, event.EventType)
		}
		if event.TargetID != "identity-registry-schema" {
			t.Errorf("Expected target identity-registry-schema, got %v", event.TargetID)
		}
		meta := responseMetadata(t, event)
		if meta["applicationId"] != "app-1" || meta["schemaId"] != "identity-registry-schema" {
			t.Errorf("Expected app-1 and identity-registry-schema in the response metadata, got %v", meta)
		}
		if fields, _ := meta["requestedFields"].([]string); len(fields) != 1 || fields[0] != "person.name" {
			t.Errorf("Expected requested field person.name, got %v", meta["requestedFields"])
		}
		if _, ok := meta["query"]; ok {
			t.Error("The provider sub-query can identify a citizen and must not be recorded")
		}
	})

	t.Run("failure", func(t *testing.T) {
		sink := &audit.CaptureSink{}
		auditor := NewAuditor(sink, "SERVICE", "orchestration-engine")

		auditor.ProviderFetch(context.Background(), fetch, nil, errors.New("connection refused"))

		event := onlyRecord(t, sink)
		if event.Status != audit.StatusFailure || responseMetadata(t, event)["error"] != "connection refused" {
			t.Errorf("Expected a failure with the provider error, got %s %v", event.Status, event.Metadata)
		}
	})
}

func TestAuditor_NilSink(t *testing.T) {
	auditor := NewAuditor(nil, "SERVICE", "orchestration-engine")

	// This should not panic or cause errors
	ctx := context.Background()
	auditor.RequestReceived(ctx, "app-1", "query { a }")
	auditor.PolicyCheck(ctx, "app-1", nil, nil, nil)
	auditor.ConsentCheck(ctx, "app-1", "", nil, nil, nil)
	auditor.ProviderFetch(ctx, federator.ProviderFetch{}, nil, nil)
}
