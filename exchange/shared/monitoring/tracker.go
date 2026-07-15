package monitoring

import "time"

// Tracker abstracts telemetry operations to avoid hard dependencies on monitoring packages
type Tracker interface {
	RecordBusinessEvent(action, outcome string)
	RecordExternalCall(target, operation string, duration time.Duration, err error)
}

// OTelTracker records real metrics via OpenTelemetry
type OTelTracker struct{}

// NewOTelTracker creates a new instance of OTelTracker
func NewOTelTracker() OTelTracker {
	return OTelTracker{}
}

func (o OTelTracker) RecordBusinessEvent(action, outcome string) {
	RecordBusinessEvent(action, outcome)
}

func (o OTelTracker) RecordExternalCall(target, operation string, duration time.Duration, err error) {
	RecordExternalCall(target, operation, duration, err)
}

// NoOpTracker acts as a no-op fallback when observability is disabled
type NoOpTracker struct{}

// NewNoOpTracker creates a new instance of NoOpTracker
func NewNoOpTracker() NoOpTracker {
	return NoOpTracker{}
}

func (n NoOpTracker) RecordBusinessEvent(action, outcome string) {}
func (n NoOpTracker) RecordExternalCall(target, operation string, duration time.Duration, err error) {}
