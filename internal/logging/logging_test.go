// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, Config{Level: "info", Format: "json"}, cfg)
	assert.NoError(t, cfg.Validate())
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "debug text", cfg: Config{Level: "debug", Format: "text"}},
		{name: "error json", cfg: Config{Level: "error", Format: "json"}},
		{name: "unknown level", cfg: Config{Level: "verbose", Format: "json"}, wantErr: "logging.level must be debug, info, warn or error"},
		{name: "empty level", cfg: Config{Format: "json"}, wantErr: "logging.level must be debug, info, warn or error"},
		{name: "uppercase level", cfg: Config{Level: "INFO", Format: "json"}, wantErr: "logging.level must be debug, info, warn or error"},
		{name: "unknown format", cfg: Config{Level: "info", Format: "xml"}, wantErr: "logging.format must be json or text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestNewLoggerJSONCarriesCommonFields(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, Config{Level: "info", Format: "json"}, "policy-decision-point", "staging", "v1.2.3")

	logger.Info("started", "port", 8082)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "INFO", line["level"])
	assert.Equal(t, "started", line["msg"])
	assert.Equal(t, "policy-decision-point", line["service"])
	assert.Equal(t, "staging", line["environment"])
	assert.Equal(t, "v1.2.3", line["version"])
	assert.EqualValues(t, 8082, line["port"])
}

func TestNewLoggerOmitsEmptyEnvironment(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, Config{Level: "info", Format: "json"}, "consent-engine", "", "dev")

	logger.Info("started")

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.NotContains(t, line, "environment")
}

func TestNewLoggerText(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, Config{Level: "info", Format: "text"}, "portal-backend", "local", "dev")

	logger.Info("started")

	out := buf.String()
	assert.Contains(t, out, "msg=started")
	assert.Contains(t, out, "service=portal-backend")
	assert.Contains(t, out, "environment=local")
	assert.False(t, json.Valid([]byte(strings.TrimSpace(out))))
}

func TestNewLoggerFiltersBelowLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, Config{Level: "warn", Format: "json"}, "orchestration-engine", "", "dev")

	logger.Info("dropped")
	logger.Warn("kept")

	assert.NotContains(t, buf.String(), "dropped")
	assert.Contains(t, buf.String(), "kept")
}

func TestSetupSetsDefault(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	logger := Setup(Config{Level: "debug", Format: "json"}, "orchestration-engine", "", "dev")

	assert.Same(t, logger, slog.Default())
	assert.True(t, slog.Default().Enabled(t.Context(), slog.LevelDebug))
}
