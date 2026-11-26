// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package audit writes audit records as structured slog records marked
// category=audit.
//
// The Orchestration Engine and the Portal Backend give LogSink a logger whose
// only handler is the OpenTelemetry log exporter from
// github.com/OpenNSW/core/telemetry. Records therefore go to the OTLP endpoint
// configured under otel and never to the process log. When log export is off,
// the services pass a nil logger and records are dropped.
//
// The record shape matches the LogSink in github.com/OpenNSW/core/platform/audit,
// so one query finds audit records from OpenNDX and from the NSW.
package audit

import (
	"context"
	"log/slog"
	"sync"
)

// Category is the value of the category attribute on every record LogSink
// writes. Log pipelines and backends filter audit records on it.
const Category = "audit"

// Status is the outcome of an audited operation.
type Status string

const (
	StatusSuccess Status = "SUCCESS"
	StatusFailure Status = "FAILURE"
)

// Record is one audit event.
type Record struct {
	EventType  string
	Action     string
	Status     Status
	ActorType  string
	ActorID    string
	TargetType string
	TargetID   string
	TraceID    string
	Metadata   map[string]any
}

// Sink receives audit records. Implementations must be safe for concurrent use.
type Sink interface {
	Write(ctx context.Context, r Record)
}

// LogSink writes records through an slog logger.
type LogSink struct {
	logger *slog.Logger
}

var _ Sink = (*LogSink)(nil)

// NewLogSink returns a LogSink that writes through logger. A nil logger
// discards every record.
func NewLogSink(logger *slog.Logger) *LogSink {
	if logger != nil {
		logger = logger.With(slog.String("category", Category))
	}
	return &LogSink{logger: logger}
}

// NewExportSink returns a LogSink that writes only to export, the OTLP log
// handler from telemetry.Provider.SlogHandler, so records stay out of the
// process log. When export is nil, log export is off: NewExportSink logs a
// warning that audit records are not retained and returns a sink that
// discards them.
func NewExportSink(export slog.Handler) *LogSink {
	if export == nil {
		slog.Warn("otel log export is off: audit records (category=" + Category + ") are not retained")
		return NewLogSink(nil)
	}
	return NewLogSink(slog.New(export))
}

// Write writes r as an info record with the message "audit". Action, TargetID,
// TraceID and Metadata are left out when empty. The record is written even if
// ctx is cancelled, since the audited operation has already happened.
func (s *LogSink) Write(ctx context.Context, r Record) {
	if s == nil || s.logger == nil {
		return
	}
	attrs := []any{
		"eventType", r.EventType,
		"status", string(r.Status),
		"actorType", r.ActorType,
		"actorID", r.ActorID,
		"targetType", r.TargetType,
	}
	if r.Action != "" {
		attrs = append(attrs, "action", r.Action)
	}
	if r.TargetID != "" {
		attrs = append(attrs, "targetID", r.TargetID)
	}
	if r.TraceID != "" {
		attrs = append(attrs, "traceID", r.TraceID)
	}
	if len(r.Metadata) > 0 {
		attrs = append(attrs, "metadata", r.Metadata)
	}
	s.logger.InfoContext(context.WithoutCancel(ctx), "audit", attrs...)
}

// CaptureSink keeps the records written to it, for tests.
type CaptureSink struct {
	mu      sync.Mutex
	records []Record
}

var _ Sink = (*CaptureSink)(nil)

// Write implements Sink.
func (c *CaptureSink) Write(_ context.Context, r Record) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r)
}

// Records returns a copy of the records written so far, in order.
func (c *CaptureSink) Records() []Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Record(nil), c.records...)
}
