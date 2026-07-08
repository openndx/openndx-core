package database

import (
	"testing"

	"github.com/gov-dx-sandbox/tests/integration/testutils"
	"github.com/stretchr/testify/assert"
)

// TestConsentEngine_DatabaseConnection tests real database connection for Consent Engine
func TestConsentEngine_DatabaseConnection(t *testing.T) {
	cleanup := testutils.WithTestDBEnv(t, testutils.DBConfig{
		Port:     "5434",
		Database: "consent_db",
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

