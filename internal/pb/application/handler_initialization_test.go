// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package application

import (
	"testing"

	"github.com/openndx/openndx-core/internal/pb/idp/idptest"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
)

// TestNewHandler checks that NewHandler wires the given dependencies.
// Configuration loading is covered by cmd/pb's config tests.
func TestNewHandler(t *testing.T) {
	db := setupSQLiteTestDB(t)
	service := NewService(db, policy.NewClient("http://localhost:9999"), &idptest.Mock{})

	handler := NewHandler(service)

	assert.NotNil(t, handler)
	assert.Same(t, service, handler.service)
}
