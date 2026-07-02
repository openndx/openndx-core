package audit

import "context"

// Auditor is the primary interface for audit logging operations.
// This interface provides a clean abstraction for audit capabilities,
// decoupling callers from the audit service implementation (https://github.com/LSFLK/argus).
//
// Implementations should handle:
// - Asynchronous logging (fire-and-forget)
// - Graceful degradation when audit service is unavailable
// - Thread-safe operations
type Auditor interface {
	// LogEvent logs an audit event asynchronously.
	// The implementation should handle the event in a background goroutine
	// to avoid blocking the calling code.
	//
	// If the audit service is disabled or unavailable, this method should
	// return immediately without error (graceful degradation).
	LogEvent(ctx context.Context, event *AuditLogRequest)

	// IsEnabled returns whether audit logging is currently enabled.
	// This can be used by callers to skip expensive audit event preparation
	// when audit logging is disabled.
	IsEnabled() bool
}

// AuditClient is an alias for Auditor to maintain backward compatibility.
// Deprecated: Use Auditor instead. This will be removed in a future version.
type AuditClient = Auditor

