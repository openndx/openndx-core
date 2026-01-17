package database

import (
	"testing"

	"github.com/gov-dx-sandbox/tests/integration/testutils"
	"github.com/stretchr/testify/assert"
)

// TestPolicyDecisionPoint_DatabaseConnection tests real database connection for Policy Decision Point
func TestPolicyDecisionPoint_DatabaseConnection(t *testing.T) {
	cleanup := testutils.WithTestDBEnv(t, testutils.DBConfig{
		Port:     "5433",
		Database: "policy_db",
		Password: "password",
	})
	defer cleanup()

	// Test that we can connect to the database
	db := testutils.SetupPostgresTestDB(t)
	if db == nil {
		t.Skip("Database connection not available")
		return
	}

	// Test connection by pinging
	sqlDB, err := db.DB()
	assert.NoError(t, err)
	assert.NoError(t, sqlDB.Ping())
}

// NOTE: Tests for ConnectGormDB with invalid configs have been removed.
// These tests require importing service packages which creates module dependency issues.
// Connection error handling is tested at the service level in unit tests.

// TestPolicyDecisionPoint_SetupPostgresTestDB_NoConnection tests that SetupPostgresTestDB handles connection failures gracefully
func TestPolicyDecisionPoint_SetupPostgresTestDB_NoConnection(t *testing.T) {
	cleanup := testutils.WithTestDBEnvFull(t, map[string]string{
		"TEST_DB_HOST":     "invalid-host-that-does-not-exist",
		"TEST_DB_PORT":     "5432",
		"TEST_DB_USERNAME": "invalid-user",
		"TEST_DB_PASSWORD": "invalid-pass",
		"TEST_DB_DATABASE": "invalid-db",
	})
	defer cleanup()

	// Should skip the test gracefully
	db := testutils.SetupPostgresTestDB(t)
	if db == nil {
		t.Skip("Database connection not available - this is expected")
	}
}

