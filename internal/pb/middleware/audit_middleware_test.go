// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package middleware

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/openndx/openndx-core/internal/audit"
	"github.com/openndx/openndx-core/internal/pb/caller/callertest"
)

// onlyRecord returns the single record sink received.
func onlyRecord(t *testing.T, sink *audit.CaptureSink) audit.Record {
	t.Helper()
	records := sink.Records()
	if len(records) != 1 {
		t.Fatalf("got %d audit records, want 1", len(records))
	}
	return records[0]
}

func TestLogAuditEvent_WritesToTheSetSink(t *testing.T) {
	sink := &audit.CaptureSink{}
	SetAuditSink(sink)
	t.Cleanup(func() { SetAuditSink(nil) })

	resourceID := "schema-1"
	LogAuditEvent(callertest.NewMemberRequest(http.MethodPost, "/api/v1/schemas", nil),
		string(ResourceTypeSchemas), &resourceID, string(AuditStatusSuccess))

	got := onlyRecord(t, sink)
	want := audit.Record{
		EventType:  "MANAGEMENT_EVENT",
		Action:     "CREATE",
		Status:     audit.StatusSuccess,
		ActorType:  string(ActorTypeMember),
		ActorID:    callertest.MemberUser.Subject,
		TargetType: "RESOURCE",
		TargetID:   "schema-1",
	}
	gotWithoutMetadata := got
	gotWithoutMetadata.Metadata = nil
	if !reflect.DeepEqual(gotWithoutMetadata, want) {
		t.Errorf("record = %+v, want %+v", gotWithoutMetadata, want)
	}
	if got.Metadata["resource"] != string(ResourceTypeSchemas) {
		t.Errorf("Metadata[resource] = %v, want %s", got.Metadata["resource"], ResourceTypeSchemas)
	}
}

func TestLogAuditEvent_WithoutSinkRecordsNothing(t *testing.T) {
	SetAuditSink(nil)

	// Must not panic.
	LogAuditEvent(callertest.NewMemberRequest(http.MethodPost, "/api/v1/schemas", nil),
		string(ResourceTypeSchemas), nil, string(AuditStatusSuccess))
}

func TestLogAudit_SkipsReadOperations(t *testing.T) {
	sink := &audit.CaptureSink{}

	LogAudit(sink, callertest.NewMemberRequest(http.MethodGet, "/api/v1/schemas", nil),
		string(ResourceTypeSchemas), nil, string(AuditStatusSuccess))

	if n := len(sink.Records()); n != 0 {
		t.Errorf("got %d audit records for a GET, want 0", n)
	}
}

func TestLogAudit_ActionAndStatus(t *testing.T) {
	tests := []struct {
		method string
		status AuditStatus
		action string
	}{
		{http.MethodPost, AuditStatusSuccess, "CREATE"},
		{http.MethodPut, AuditStatusFailure, "UPDATE"},
		{http.MethodPatch, AuditStatusSuccess, "UPDATE"},
		{http.MethodDelete, AuditStatusFailure, "DELETE"},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			sink := &audit.CaptureSink{}

			LogAudit(sink, callertest.NewMemberRequest(tt.method, "/api/v1/schemas/schema-1", nil),
				string(ResourceTypeSchemas), nil, string(tt.status))

			got := onlyRecord(t, sink)
			if got.Action != tt.action || got.Status != audit.Status(tt.status) {
				t.Errorf("action/status = %s/%s, want %s/%s", got.Action, got.Status, tt.action, tt.status)
			}
			if got.TargetID != "" {
				t.Errorf("TargetID = %q, want empty for a nil resource ID", got.TargetID)
			}
		})
	}
}

func TestLogAudit_RecordsSubjectAndClientID(t *testing.T) {
	sink := &audit.CaptureSink{}

	LogAudit(sink, callertest.NewMemberRequest(http.MethodPost, "/api/v1/schemas", nil), "SCHEMAS", nil, string(AuditStatusSuccess))

	event := onlyRecord(t, sink)
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
