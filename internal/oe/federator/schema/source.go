// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package schema supplies the consumer schema the federator plans queries against.
package schema

import (
	"context"
	"fmt"
	"os"
)

// Source serves the consumer schema. It is compiled once and doesn't change while
// the OE runs: to change the schema, deploy a new schema file and restart.
type Source struct {
	compiled *CompiledSchema
}

// Load reads and compiles the consumer schema file at path.
func Load(path string) (*Source, error) {
	sdl, err := os.ReadFile(path) //nolint:gosec // schemaPath from the operator's config file
	if err != nil {
		return nil, fmt.Errorf("read schema file: %w", err)
	}
	src, err := FromSDL(string(sdl))
	if err != nil {
		return nil, fmt.Errorf("compile schema file %s: %w", path, err)
	}
	return src, nil
}

// FromSDL compiles sdl as the consumer schema.
func FromSDL(sdl string) (*Source, error) {
	compiled, err := Compile(sdl)
	if err != nil {
		return nil, err
	}
	return &Source{compiled: compiled}, nil
}

// Active returns the consumer schema. It is shared by all requests and must not
// be modified.
func (s *Source) Active(_ context.Context) (*CompiledSchema, error) {
	return s.compiled, nil
}
