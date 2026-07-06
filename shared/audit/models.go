package audit

import (
	"encoding/json"
)

// AuditLogRequest represents the request payload for creating an audit log
// Services like orchestration-engine and portal-backend can use this without importing the audit service (https://github.com/LSFLK/argus)
type AuditLogRequest struct {
	// Trace & Correlation
	TraceID *string `json:"traceId,omitempty"` // UUID string, nullable for standalone events

	// Temporal
	Timestamp string `json:"timestamp"` // ISO 8601 format, required

	// Event Classification
	EventType   *string `json:"eventType,omitempty"`   // POLICY_CHECK, MANAGEMENT_EVENT (user-defined custom names)
	EventAction *string `json:"eventAction,omitempty"` // CREATE, READ, UPDATE, DELETE
	Status      string  `json:"status"`                // SUCCESS, FAILURE

	// Actor Information (unified approach)
	ActorType string `json:"actorType"` // SERVICE, ADMIN, MEMBER, SYSTEM
	ActorID   string `json:"actorId"`   // email, uuid, or service-name (required)

	// Target Information (unified approach)
	TargetType string  `json:"targetType"`         // SERVICE, RESOURCE
	TargetID   *string `json:"targetId,omitempty"` // resource_id or service_name

	// Metadata (Payload without PII/sensitive data)
	RequestMetadata    json.RawMessage `json:"requestMetadata,omitempty"`    // Request payload without PII/sensitive data
	ResponseMetadata   json.RawMessage `json:"responseMetadata,omitempty"`   // Response or Error details
	AdditionalMetadata json.RawMessage `json:"additionalMetadata,omitempty"` // Additional context-specific data
}

// Audit log status constants
const (
	StatusSuccess = "SUCCESS"
	StatusFailure = "FAILURE"
)
