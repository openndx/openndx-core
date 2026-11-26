// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeOne writes r through a LogSink backed by a JSON handler and returns the
// decoded line.
func writeOne(t *testing.T, ctx context.Context, r Record) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	NewLogSink(slog.New(slog.NewJSONHandler(&buf, nil))).Write(ctx, r)
	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), "output: %s", buf.String())
	return line
}

func TestLogSink_WritesEveryField(t *testing.T) {
	line := writeOne(t, context.Background(), Record{
		EventType:  "POLICY_CHECK",
		Action:     "READ",
		Status:     StatusFailure,
		ActorType:  "SERVICE",
		ActorID:    "orchestration-engine",
		TargetType: "SERVICE",
		TargetID:   "policy-decision-point",
		TraceID:    "trace-1",
		Metadata:   map[string]any{"applicationId": "app-1"},
	})

	assert.Equal(t, "INFO", line["level"])
	assert.Equal(t, "audit", line["msg"])
	assert.Equal(t, Category, line["category"])
	assert.Equal(t, "POLICY_CHECK", line["eventType"])
	assert.Equal(t, "READ", line["action"])
	assert.Equal(t, "FAILURE", line["status"])
	assert.Equal(t, "SERVICE", line["actorType"])
	assert.Equal(t, "orchestration-engine", line["actorID"])
	assert.Equal(t, "SERVICE", line["targetType"])
	assert.Equal(t, "policy-decision-point", line["targetID"])
	assert.Equal(t, "trace-1", line["traceID"])
	assert.Equal(t, map[string]any{"applicationId": "app-1"}, line["metadata"])
}

func TestLogSink_OmitsEmptyOptionalFields(t *testing.T) {
	line := writeOne(t, context.Background(), Record{
		EventType:  "DATA_REQUEST",
		Status:     StatusSuccess,
		ActorType:  "APPLICATION",
		ActorID:    "app-1",
		TargetType: "SERVICE",
	})

	for _, key := range []string{"action", "targetID", "traceID", "metadata"} {
		assert.NotContains(t, line, key)
	}
	assert.Equal(t, "SUCCESS", line["status"])
}

func TestLogSink_WritesAfterContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	line := writeOne(t, ctx, Record{EventType: "PROVIDER_FETCH", Status: StatusSuccess})

	assert.Equal(t, "PROVIDER_FETCH", line["eventType"])
}

func TestLogSink_NilLoggerDiscards(t *testing.T) {
	assert.NotPanics(t, func() {
		NewLogSink(nil).Write(context.Background(), Record{EventType: "DATA_REQUEST"})
		var s *LogSink
		s.Write(context.Background(), Record{EventType: "DATA_REQUEST"})
	})
}

func TestNewExportSink_WritesOnlyToTheExportHandler(t *testing.T) {
	var export bytes.Buffer
	sink := NewExportSink(slog.NewJSONHandler(&export, nil))

	sink.Write(context.Background(), Record{EventType: "DATA_REQUEST", Status: StatusSuccess})

	var line map[string]any
	require.NoError(t, json.Unmarshal(export.Bytes(), &line))
	assert.Equal(t, Category, line["category"])
	assert.Equal(t, "DATA_REQUEST", line["eventType"])
}

func TestNewExportSink_WarnsAndDiscardsWhenExportIsOff(t *testing.T) {
	var process bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&process, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	sink := NewExportSink(nil)
	assert.Contains(t, process.String(), "audit records (category=audit) are not retained")

	process.Reset()
	sink.Write(context.Background(), Record{EventType: "DATA_REQUEST", Status: StatusSuccess})
	assert.Empty(t, process.String(), "a record must not reach the process log")
}

func TestCaptureSink_RecordsInOrder(t *testing.T) {
	var c CaptureSink
	c.Write(context.Background(), Record{EventType: "A"})
	c.Write(context.Background(), Record{EventType: "B"})

	got := c.Records()
	require.Len(t, got, 2)
	assert.Equal(t, "A", got[0].EventType)
	assert.Equal(t, "B", got[1].EventType)

	got[0].EventType = "changed"
	assert.Equal(t, "A", c.Records()[0].EventType, "Records returns a copy")
}
