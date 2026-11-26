// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package middleware

import (
	"log/slog"
	"net/http"

	"github.com/openndx/openndx-core/internal/audit"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/pb/caller"
)

// auditSink receives the Portal Backend's audit records. main sets it once with
// SetAuditSink before serving; while it is nil, nothing is recorded.
var auditSink audit.Sink

// SetAuditSink sets the sink LogAuditEvent writes to. Call it once at startup,
// before the server handles requests.
func SetAuditSink(sink audit.Sink) {
	auditSink = sink
}

// LogAudit records a MANAGEMENT_EVENT for a portal-backend write request (POST,
// PUT, PATCH or DELETE) in sink. Other methods and a nil sink record nothing.
// status is AuditStatusSuccess or AuditStatusFailure.
func LogAudit(sink audit.Sink, r *http.Request, resource string, resourceID *string, status string) {
	if sink == nil {
		return
	}

	// Only log write operations (POST, PUT, PATCH, DELETE)
	if !isWriteOperation(r.Method) {
		return
	}

	// Extract actor info directly from request
	actorType, actorIDPtr, _ := extractActorInfoFromRequest(r)
	if actorIDPtr == nil {
		// If no actor ID, we can't log the event (required field)
		slog.WarnContext(r.Context(), "Cannot log audit event: no actor ID found")
		return
	}

	// Determine event action from HTTP method (CREATE, UPDATE, DELETE)
	eventAction := determineEventType(r.Method)
	if eventAction == "" {
		return
	}

	var targetID string
	if resourceID != nil {
		targetID = *resourceID
	}

	sink.Write(r.Context(), audit.Record{
		EventType:  "MANAGEMENT_EVENT",
		Action:     eventAction,
		Status:     audit.Status(status),
		ActorType:  actorType,
		ActorID:    *actorIDPtr,
		TargetType: "RESOURCE",
		TargetID:   targetID,
		Metadata: map[string]any{
			"resource": resource,
			// The OAuth2 client the actor's token was issued to; "" when unauthenticated
			"clientId": authn.GetAuthContext(r.Context()).ClientID(),
		},
	})
}

// extractActorInfoFromRequest extracts actor information from the request
// Returns actorType, actorID, and actorRole for audit logging
// Security: Uses SYSTEM as default for unauthenticated/unknown roles to prevent privilege escalation
func extractActorInfoFromRequest(r *http.Request) (actorType string, actorID *string, actorRole *string) {
	// Try to get the principal internal/authn validated first
	authCtx := authn.GetAuthContext(r.Context())
	if authCtx == nil {
		// Unauthenticated request: use SYSTEM actor type with request identifier
		// This prevents misclassification and ensures unauthenticated actions are clearly marked
		systemActorID := "unauthenticated-request"
		systemActorType := string(ActorTypeSystem)
		slog.WarnContext(r.Context(), "Audit log for unauthenticated request - using SYSTEM actor type",
			"path", r.URL.Path,
			"method", r.Method,
			"remote_addr", r.RemoteAddr)
		return systemActorType, &systemActorID, &systemActorType
	}

	// Authenticated caller found - the token's subject is the actor
	subject := authCtx.Subject()
	actorID = &subject

	// Classify the actor by what its token may do rather than by role name:
	// onboarding members is an administrator's job, a caller in an
	// organization unit acts for that member, and anything else is a service.
	var actorTypeConst ActorType
	switch {
	case caller.HasScope(r.Context(), caller.ScopeMemberWriteAll):
		actorTypeConst = ActorTypeAdmin
	case authCtx.OUID() != "":
		actorTypeConst = ActorTypeMember
	default:
		actorTypeConst = ActorTypeSystem
	}

	// Convert to string for both actorType and actorRole
	roleStr := string(actorTypeConst)
	actorType = roleStr
	actorRole = &roleStr
	return
}

// LogAuditEvent records an audit event for r in the sink set with SetAuditSink.
// See LogAudit.
func LogAuditEvent(r *http.Request, resource string, resourceID *string, status string) {
	LogAudit(auditSink, r, resource, resourceID, status)
}

// Helper functions
func isWriteOperation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func determineEventType(method string) string {
	switch method {
	case http.MethodPost:
		return "CREATE"
	case http.MethodPut, http.MethodPatch:
		return "UPDATE"
	case http.MethodDelete:
		return "DELETE"
	default:
		return ""
	}
}
