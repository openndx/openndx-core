package middleware

import (
	"context"
	"sync"

	auditpkg "github.com/LSFLK/argus/pkg/audit"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/logger"
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/pkg/graphql"
	"github.com/OpenNDX/openndx-core/exchange/shared/monitoring"
	"github.com/google/uuid"
)

// auditConfig holds the audit configuration values
var (
	auditConfig struct {
		actorType string
		actorID   string
	}
	auditConfigOnce sync.Once
)

// InitializeAuditConfig initializes the audit configuration from config values
// This should be called once during application startup from main.go
// Values are read from config.json via config.AuditConfig
// Note: targetType is not stored here as it varies per API call
func InitializeAuditConfig(actorType, actorID string) {
	auditConfigOnce.Do(func() {
		// These values are expected to be pre-populated with defaults from the configs package.
		auditConfig.actorType = actorType
		auditConfig.actorID = actorID
	})
}

// getAuditActorType returns the configured actor type
// InitializeAuditConfig guarantees this is always set (with default if needed)
func getAuditActorType() string {
	return auditConfig.actorType
}

// getAuditActorID returns the configured actor ID
// InitializeAuditConfig guarantees this is always set (with default if needed)
func getAuditActorID() string {
	return auditConfig.actorID
}

// maxAuditErrorsToLog is the maximum number of errors to include in audit log metadata
// This limit prevents audit logs from becoming too large when there are many errors
const maxAuditErrorsToLog = 3

// Context key for audit metadata
type contextKey string

const auditMetadataKey contextKey = "auditMetadata"

// Metadata holds metadata needed for audit logging in orchestration-engine
type Metadata struct {
	ConsumerAppID    string
	ProviderFieldMap *[]ProviderLevelFieldRecord
}

// ProviderLevelFieldRecord represents a field record for provider-level operations
// This is imported from federator package context
type ProviderLevelFieldRecord struct {
	SchemaId   string
	ServiceKey string
	FieldPath  string
}

// NewContextWithMetadata creates a new context with audit metadata
func NewContextWithMetadata(ctx context.Context, metadata *Metadata) context.Context {
	return context.WithValue(ctx, auditMetadataKey, metadata)
}

// MetadataFromContext retrieves audit metadata from context
func MetadataFromContext(ctx context.Context) *Metadata {
	metadata, ok := ctx.Value(auditMetadataKey).(*Metadata)
	if !ok {
		return nil
	}
	return metadata
}

// LogAuditEvent is a shared helper function that handles common audit logging logic:
// - Gets/ensures traceID in context
// - Marshals metadata (request or response)
// - Creates AuditLogRequest struct
// - Logs the event asynchronously
// Returns the updated context with traceID (if one was generated) to ensure trace correlation
// targetType should be determined per API call (e.g., "SERVICE" for service-to-service calls, "RESOURCE" for resource operations)
func LogAuditEvent(ctx context.Context, eventType string, targetID *string, targetType string, requestMetadata map[string]interface{}, responseMetadata map[string]interface{}, status string) context.Context {
	// Get or generate traceID
	traceID := monitoring.GetTraceIDFromContext(ctx)
	if traceID == "" {
		traceID = uuid.New().String()
		ctx = monitoring.WithTraceID(ctx, traceID)
	}

	// Use configured audit fields from config.json (with fallback defaults for safety)
	// These are initialized in main.go via InitializeAuditConfig() from config.AuditConfig
	actorType := getAuditActorType()
	actorID := getAuditActorID()

	meta := make(map[string]interface{})
	if len(requestMetadata) > 0 {
		meta["requestMetadata"] = requestMetadata
	}
	if len(responseMetadata) > 0 {
		meta["responseMetadata"] = responseMetadata
	}

	// Create audit request
	auditRequest := &auditpkg.AuditLogRequest{
		TraceID:    &traceID,
		Timestamp:  auditpkg.CurrentTimestamp(),
		EventType:  eventType,
		Status:     status,
		ActorType:  actorType,
		ActorID:    actorID,
		TargetType: targetType,
		TargetID:   targetID,
		Metadata:   meta,
	}

	// Log the audit event asynchronously using the global audit package
	auditpkg.LogAuditEvent(ctx, auditRequest)

	// Return the updated context to ensure traceID correlation across the request flow
	return ctx
}

// FederationServiceRequest represents a service request for audit logging
type FederationServiceRequest struct {
	ServiceKey     string
	SchemaID       string
	GraphQLRequest graphql.Request
}

// LogProviderFetch logs a provider fetch event to the audit service asynchronously
func LogProviderFetch(ctx context.Context, providerSchemaID string, req *FederationServiceRequest, response *graphql.Response, err error) {
	// Retrieve metadata from context
	metadata := MetadataFromContext(ctx)
	if metadata == nil {
		logger.Log.Warn("Audit metadata missing from context, skipping audit log")
		return
	}

	// Extract requested fields for this provider
	requestedFields := make([]string, 0)
	if metadata.ProviderFieldMap != nil {
		for _, field := range *metadata.ProviderFieldMap {
			if field.SchemaId == req.SchemaID && field.ServiceKey == req.ServiceKey {
				requestedFields = append(requestedFields, field.FieldPath)
			}
		}
	}

	// Combine requested data and additional info into response metadata
	// (since we're logging after receiving the response)
	responseMetadata := map[string]interface{}{
		"applicationId":   metadata.ConsumerAppID,
		"schemaId":        providerSchemaID,
		"requestedFields": requestedFields,
		"query":           req.GraphQLRequest.Query,
		"serviceKey":      req.ServiceKey,
	}
	if err != nil {
		responseMetadata["error"] = err.Error()
	}
	if response != nil {
		responseMetadata["hasErrors"] = len(response.Errors) > 0
		if len(response.Errors) > 0 {
			responseMetadata["errorCount"] = len(response.Errors)
			// Include first few errors (limit to avoid large payloads)
			errorDetails := make([]interface{}, 0)
			for i, gqlErr := range response.Errors {
				if i >= maxAuditErrorsToLog {
					break
				}
				errorDetails = append(errorDetails, gqlErr)
			}
			responseMetadata["errors"] = errorDetails
		}
		if response.Data != nil {
			// Include data keys for reference (not full data to avoid large payloads)
			dataKeys := make([]string, 0, len(response.Data))
			for key := range response.Data {
				dataKeys = append(dataKeys, key)
			}
			responseMetadata["dataKeys"] = dataKeys
		}
	}

	auditStatus := auditpkg.StatusSuccess
	if err != nil || (response != nil && len(response.Errors) > 0) {
		auditStatus = auditpkg.StatusFailure
	}

	// Use shared helper function to log audit event
	// Update context with traceID if one was generated
	// Providers are services, so targetType is "SERVICE"
	ctx = LogAuditEvent(ctx, "PROVIDER_FETCH", &req.ServiceKey, "SERVICE", nil, responseMetadata, auditStatus)
}

// LogRequestReceived logs a request received event to the audit service asynchronously
func LogRequestReceived(ctx context.Context, eventType string, actorType string, actorId string, requestMetadata map[string]interface{}) context.Context {
	// Get or generate traceID
	traceID := monitoring.GetTraceIDFromContext(ctx)
	if traceID == "" {
		traceID = uuid.New().String()
		ctx = monitoring.WithTraceID(ctx, traceID)
	}
	status := auditpkg.StatusSuccess

	targetID := "SERVICE"
	meta := make(map[string]interface{})
	if len(requestMetadata) > 0 {
		meta["requestMetadata"] = requestMetadata
	}

	// Create audit request
	auditRequest := &auditpkg.AuditLogRequest{
		TraceID:    &traceID,
		Timestamp:  auditpkg.CurrentTimestamp(),
		EventType:  eventType,
		Status:     status,
		ActorType:  actorType,
		ActorID:    actorId,
		TargetType: "SERVICE",
		TargetID:   &targetID,
		Metadata:   meta,
	}

	// Log the audit event asynchronously using the global audit package
	auditpkg.LogAuditEvent(ctx, auditRequest)

	// Return the updated context to ensure traceID correlation across the request flow
	return ctx
}
