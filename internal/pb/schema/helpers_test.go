// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package schema

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/openndx/openndx-core/internal/pb/database/dbtest"
	"github.com/openndx/openndx-core/internal/pb/member"
	"github.com/openndx/openndx-core/internal/pb/policy"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// MockRoundTripper is a mock implementation of http.RoundTripper
type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

// RoundTrip executes the mock RoundTripFunc
func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

// setupSQLiteTestDB creates an in-memory SQLite database with the member and
// schema models migrated.
func setupSQLiteTestDB(t *testing.T) *gorm.DB {
	return dbtest.SetupSQLiteDB(t, &member.Member{}, &Schema{}, &SchemaSubmission{})
}

// newTestHandler creates a schema Handler whose PDP client points at an
// unreachable address, so PDP-dependent operations fail.
func newTestHandler(db *gorm.DB) *Handler {
	pdpClient := policy.NewClient("http://localhost:8082")
	return NewHandler(NewService(db, pdpClient))
}

// testHandlerEnv bundles a schema Handler with its test database
type testHandlerEnv struct {
	*testing.T
	db      *gorm.DB
	handler *Handler
}

// newTestHandlerEnv creates a schema Handler backed by an in-memory SQLite database
func newTestHandlerEnv(t *testing.T) *testHandlerEnv {
	db := setupSQLiteTestDB(t)
	return &testHandlerEnv{T: t, db: db, handler: newTestHandler(db)}
}

// createTestMember creates a member in the database for testing (bypasses IDP)
func createTestMember(t *testing.T, db *gorm.DB, email string) string {
	m := member.Member{
		MemberID:    "mem_" + fmt.Sprintf("%d", uniqueID()),
		Name:        "Test Member",
		Email:       email,
		PhoneNumber: "1234567890",
		OuID:        "ou-" + fmt.Sprintf("%d", uniqueID()),
	}
	err := db.Create(&m).Error
	assert.NoError(t, err)
	return m.MemberID
}

// createTestSchema creates a schema in the database for testing (bypasses async creation)
func createTestSchema(t *testing.T, db *gorm.DB, memberID string) string {
	schema := Schema{
		SchemaID:   "schema_" + fmt.Sprintf("%d", uniqueID()),
		SchemaName: "Test Schema",
		SDL:        "type Query { test: String }",
		Endpoint:   "http://example.com/graphql",
		MemberID:   memberID,
	}
	err := db.Create(&schema).Error
	assert.NoError(t, err)
	return schema.SchemaID
}

// testIDs backs uniqueID.
var testIDs atomic.Int64

// uniqueID returns a number no other call in this test binary returns, for
// building unique test IDs and emails. uniqueID() isn't unique:
// on Windows, consecutive calls can return the same value.
func uniqueID() int64 {
	return testIDs.Add(1)
}
