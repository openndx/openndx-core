package middleware

import (
	"log/slog"
	"net/http"

	"github.com/OpenDIF/opendif-core/shared/audit"
	"github.com/gov-dx-sandbox/portal-backend/v1/models"
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
		slog.Warn("Cannot log audit event: no actor ID found")
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
	eventType := &managementEventType

	// Set target type and ID
	targetType := "RESOURCE"

	// Create audit event using shared/audit DTO
	// Use shared utilities for timestamp and metadata marshaling
	timestamp := audit.CurrentTimestamp()
	additionalMetadata := audit.MarshalMetadata(map[string]interface{}{
		"resource":   resource,
		"resourceId": resourceID,
	})

	auditRequest := &audit.AuditLogRequest{
		TraceID:            nil, // No trace ID for standalone management events
		Timestamp:          timestamp,
		EventType:          eventType,
		EventAction:        &eventAction,
		Status:             status,
		ActorType:          actorType,
		ActorID:            actorID,
		TargetType:         targetType,
		AdditionalMetadata: additionalMetadata,
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
	// Try to get authenticated user first
	user, err := GetUserFromRequest(r)
	if err != nil || user == nil {
		// Unauthenticated request: use SYSTEM actor type with request identifier
		// This prevents misclassification and ensures unauthenticated actions are clearly marked
		systemActorID := "unauthenticated-request"
		systemActorType := string(models.ActorTypeSystem)
		slog.Warn("Audit log for unauthenticated request - using SYSTEM actor type",
			"path", r.URL.Path,
			"method", r.Method,
			"remote_addr", r.RemoteAddr)
		return systemActorType, &systemActorID, &systemActorType
	}

	// Authenticated user found - extract actor information
	userID := user.IdpUserID
	actorID = &userID

	// Map user's primary role to actor type
	primaryRole := user.GetPrimaryRole()
	var actorTypeConst models.ActorType

	switch primaryRole {
	case models.RoleAdmin:
		actorTypeConst = models.ActorTypeAdmin
	case models.RoleMember:
		actorTypeConst = models.ActorTypeMember
	case models.RoleSystem:
		actorTypeConst = models.ActorTypeSystem
	default:
		// Security: Use SYSTEM for unknown roles instead of MEMBER to prevent privilege escalation
		// Unknown roles should be investigated and properly mapped
		actorTypeConst = models.ActorTypeSystem
		slog.Warn("Unknown role encountered in audit log - using SYSTEM actor type as safe default",
			"user_id", userID,
			"primary_role", primaryRole,
			"all_roles", user.Roles,
			"path", r.URL.Path,
			"method", r.Method)
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
		slog.Warn("Audit logging skipped: globalAuditMiddleware is not initialized")
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
