// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package middleware

import (
	"log/slog"
	"net/http"

	"github.com/LSFLK/argus/pkg/audit"
	"github.com/openndx/openndx-core/internal/authn"
	"github.com/openndx/openndx-core/internal/pb/caller"
)

// LogAudit logs an audit event for portal-backend operations by extracting request info and creating an audit log
func LogAudit(client audit.Auditor, r *http.Request, resource string, resourceID *string, status string) {
	// Skip if audit client is not set or not enabled
	if client == nil || !client.IsEnabled() {
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
	actorID := *actorIDPtr

	// Determine event action from HTTP method (CREATE, UPDATE, DELETE)
	eventAction := determineEventType(r.Method)
	if eventAction == "" {
		return
	}

	// Set event type to MANAGEMENT_EVENT for portal operations
	managementEventType := "MANAGEMENT_EVENT"

	// Set target type and ID
	targetType := "RESOURCE"

	// Create audit event using argus DTO
	timestamp := audit.CurrentTimestamp()

	auditRequest := &audit.AuditLogRequest{
		TraceID:    nil, // No trace ID for standalone management events
		Timestamp:  timestamp,
		EventType:  managementEventType,
		Action:     eventAction,
		Status:     status,
		ActorType:  actorType,
		ActorID:    actorID,
		TargetType: targetType,
		TargetID:   resourceID,
		Metadata: map[string]interface{}{
			"resource": resource,
			// The OAuth2 client the actor's token was issued to; "" when unauthenticated
			"clientId": authn.GetAuthContext(r.Context()).ClientID(),
		},
	}

	// Log asynchronously (fire-and-forget). Pass the request context so tracing
	// metadata can propagate; the Auditor implementation is responsible for
	// detaching from request cancellation as needed.
	client.LogEvent(r.Context(), auditRequest)
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

// LogAuditEvent - global function for easy access from handlers
func LogAuditEvent(r *http.Request, resource string, resourceID *string, status string) {
	globalMiddleware := audit.GetGlobalAuditMiddleware()
	if globalMiddleware != nil {
		LogAudit(globalMiddleware.Client(), r, resource, resourceID, status)
	} else {
		slog.WarnContext(r.Context(), "Audit logging skipped: globalAuditMiddleware is not initialized")
	}
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
