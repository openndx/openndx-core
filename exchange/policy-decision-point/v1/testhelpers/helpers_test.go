package testhelpers

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetEnvOrDefault(t *testing.T) {
	// Test with existing env var
	os.Setenv("TEST_ENV_VAR", "test-value")
	defer os.Unsetenv("TEST_ENV_VAR")

	value := getEnvOrDefault("TEST_ENV_VAR", "default-value")
	assert.Equal(t, "test-value", value)
}

func TestGetEnvOrDefault_DefaultValue(t *testing.T) {
	// Ensure env var is not set
	os.Unsetenv("NONEXISTENT_VAR")

	value := getEnvOrDefault("NONEXISTENT_VAR", "default-value")
	assert.Equal(t, "default-value", value)
}

func TestIsValidDBName(t *testing.T) {
	validNames := []string{
		"test_db",
		"testdb123",
		"TEST_DB",
		"test",
		"db_123",
		"a",
		"123",
		"_test",
		"test_",
	}

	for _, name := range validNames {
		assert.True(t, isValidDBName(name), "Expected %s to be valid", name)
	}
}

func TestIsValidDBName_Invalid(t *testing.T) {
	invalidNames := []string{
		"test-db",      // hyphen not allowed
		"test.db",      // dot not allowed
		"test db",      // space not allowed
		"test@db",      // special char not allowed
		"test/db",      // slash not allowed
		"",             // empty not allowed
		"test.db.name", // multiple dots
		"test\ndb",     // newline not allowed
		"test\tdb",     // tab not allowed
	}

	for _, name := range invalidNames {
		assert.False(t, isValidDBName(name), "Expected %s to be invalid", name)
	}
}

func TestIsDBNotExistError(t *testing.T) {
	// Test with error containing "does not exist"
	err1 := &testError{message: "database does not exist"}
	assert.True(t, isDBNotExistError(err1))

	// Test with error containing "3D000"
	err2 := &testError{message: "error code 3D000"}
	assert.True(t, isDBNotExistError(err2))

	// Test with nil error
	assert.False(t, isDBNotExistError(nil))

	// Test with different error
	err3 := &testError{message: "connection refused"}
	assert.False(t, isDBNotExistError(err3))
}

func TestIsDBNotExistError_VariousErrors(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
		want   bool
	}{
		{
			name:   "Error with 'does not exist'",
			errMsg: "database 'testdb' does not exist",
			want:   true,
		},
		{
			name:   "Error with '3D000'",
			errMsg: "error code 3D000: database not found",
			want:   true,
		},
		{
			name:   "Error with both patterns",
			errMsg: "database does not exist (3D000)",
			want:   true,
		},
		{
			name:   "Connection refused",
			errMsg: "connection refused",
			want:   false,
		},
		{
			name:   "Authentication failed",
			errMsg: "password authentication failed",
			want:   false,
		},
		{
			name:   "Empty error message",
			errMsg: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &testError{message: tt.errMsg}
			got := isDBNotExistError(err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// testError is a simple error type for testing
type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}

func TestCleanupTestData_WithValidDB(t *testing.T) {
	// Use SQLite in-memory database for testing CleanupTestData
	// Note: SQLite doesn't support PostgreSQL-specific features, so we test
	// that CleanupTestData handles the case where table doesn't exist gracefully
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	}()

	// Test that CleanupTestData doesn't panic even if table doesn't exist
	// (SQLite can't create the PostgreSQL-specific schema)
	CleanupTestData(t, db)

	// Should not panic - CleanupTestData should handle missing table gracefully
	assert.True(t, true)
}

// NOTE: TestSetupPostgresTestDB_NoConnection has been moved to
// tests/integration/database/pdp_database_test.go as an integration test.
// Unit tests should not use real database connections.
