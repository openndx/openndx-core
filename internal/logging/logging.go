// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package logging sets up the slog logger every service writes through.
//
// main calls Setup once, right after loading its configuration. All other code
// logs through the slog default, using the ...Context variants (slog.InfoContext,
// slog.ErrorContext, ...) whenever a context is available, so request-scoped
// fields such as the trace ID reach the log line.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// Config is the logging section of a service's config file.
type Config struct {
	// Level is debug, info, warn or error.
	Level string `yaml:"level"`
	// Format is json or text.
	Format string `yaml:"format"`
}

// DefaultConfig returns the configuration used for keys the config file leaves out.
func DefaultConfig() Config {
	return Config{Level: "info", Format: "json"}
}

// Validate reports an unknown level or format.
func (c Config) Validate() error {
	if _, err := parseLevel(c.Level); err != nil {
		return err
	}
	switch c.Format {
	case "json", "text":
		return nil
	default:
		return fmt.Errorf("logging.format must be json or text")
	}
}

// Setup builds the logger from cfg, sets it as slog's default and returns it.
// Every line carries the service and version, and the environment when it is
// not empty. cfg must be valid.
func Setup(cfg Config, service, environment, version string) *slog.Logger {
	logger := newLogger(os.Stdout, cfg, service, environment, version)
	slog.SetDefault(logger)
	return logger
}

// newLogger builds the logger Setup installs, writing to w.
func newLogger(w io.Writer, cfg Config, service, environment, version string) *slog.Logger {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}

	attrs := []any{"service", service, "version", version}
	if environment != "" {
		attrs = append(attrs, "environment", environment)
	}
	return slog.New(handler).With(attrs...)
}

// parseLevel maps a configured level name to its slog level.
func parseLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logging.level must be debug, info, warn or error")
	}
}
