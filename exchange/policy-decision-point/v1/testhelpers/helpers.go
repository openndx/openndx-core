package testhelpers

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gov-dx-sandbox/exchange/policy-decision-point/v1/models"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// StringPtr returns a pointer to the given string value.
func StringPtr(s string) *string {
	return &s
}

// OwnerPtr returns a pointer to the given Owner value.
func OwnerPtr(o models.Owner) *models.Owner {
	return &o
}

// SetupTestDB creates an in-memory SQLite database for unit testing.
// It creates the policy_metadata table with SQLite-compatible schema.
// SQLite doesn't support PostgreSQL-specific features like gen_random_uuid(), enums, jsonb.
// This is fast and doesn't require a real database connection.
//
// For integration-style tests that need real PostgreSQL behavior (transactions, complex queries),
// use SetupPostgresTestDB instead.
func SetupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Create table manually for SQLite compatibility
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS policy_metadata (
			id TEXT PRIMARY KEY,
			schema_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			display_name TEXT,
			description TEXT,
			source TEXT NOT NULL DEFAULT 'fallback',
			is_owner INTEGER NOT NULL DEFAULT 0,
			access_control_type TEXT NOT NULL DEFAULT 'restricted',
			allow_list TEXT NOT NULL DEFAULT '{}',
			owner TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(schema_id, field_name)
		)
	`
	if err := db.Exec(createTableSQL).Error; err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	return db
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isValidDBName checks if the database name is safe to use in SQL
func isValidDBName(name string) bool {
	match, _ := regexp.MatchString("^[a-zA-Z0-9_]+$", name)
	return match
}

// SetupPostgresTestDB creates a PostgreSQL test database connection for integration tests.
// This function should ONLY be used in integration tests or tests that explicitly require
// real database behavior (e.g., testing transactions, complex queries, or GORM-specific features).
//
// All database testing should be done via integration tests in tests/integration/.
//
// This function will skip the test if a database connection cannot be established.
func SetupPostgresTestDB(t *testing.T) *gorm.DB {
	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5432")
	testDB := getEnvOrDefault("TEST_DB_DATABASE", "pdp_service_test")
	sslmode := getEnvOrDefault("TEST_DB_SSLMODE", "disable")

	// Try credential combinations
	credentials := []struct {
		user string
		pass string
	}{
		{getEnvOrDefault("TEST_DB_USERNAME", "postgres"), getEnvOrDefault("TEST_DB_PASSWORD", "password")},
		{"postgres", "password"},
		{"postgres", ""},
		{os.Getenv("USER"), ""},
	}

	var db *gorm.DB
	var err error

	for _, cred := range credentials {
		if cred.user == "" {
			continue
		}

		// 1. Try connecting to the test database directly
		db, err = tryConnection(host, port, cred.user, cred.pass, testDB, sslmode)
		if err == nil {
			t.Logf("Connected to PostgreSQL with user=%s", cred.user)
			return setupDatabase(t, db)
		}

		// 2. If test database doesn't exist, try to connect to default database and create it
		if isDBNotExistError(err) {
			defaultDB := "postgres"
			if adminDB, adminErr := tryConnection(host, port, cred.user, cred.pass, defaultDB, sslmode); adminErr == nil {
				t.Logf("Connected to admin database, creating test database")

				// SECURITY NOTE: CREATE DATABASE cannot be parameterized in PostgreSQL, requiring
				// string formatting which is inherently risky for SQL injection. To mitigate this risk:
				// 1. We validate both testDB and cred.user with isValidDBName() using strict regex ^[a-zA-Z0-9_]+$
				// 2. We double-quote identifiers to prevent keyword conflicts
				// 3. The validation ensures only alphanumeric characters and underscores are allowed
				// This pattern should be used with extreme caution and only in test code with controlled inputs.
				if !isValidDBName(testDB) {
					t.Fatalf("Invalid database name: %s", testDB)
				}
				if !isValidDBName(cred.user) {
					t.Fatalf("Invalid database owner: %s", cred.user)
				}

				// Create test database using validated inputs
				createSQL := fmt.Sprintf("CREATE DATABASE \"%s\" WITH OWNER = \"%s\"", testDB, cred.user)
				if createErr := adminDB.Exec(createSQL).Error; createErr != nil {
					// Database might already exist (race condition), ignore error
					t.Logf("Note: Could not create test database: %v", createErr)
				}

				// Close admin connection properly
				if sqlDB, err := adminDB.DB(); err == nil {
					sqlDB.Close()
				}

				// Now try connecting to the test database again
				db, err = tryConnection(host, port, cred.user, cred.pass, testDB, sslmode)
				if err == nil {
					t.Logf("Successfully created and connected to test database with user=%s", cred.user)
					return setupDatabase(t, db)
				}
			}
		}
	}

	if err != nil {
		t.Skipf("Skipping test: could not connect to test database with any credentials: %v", err)
		return nil
	}

	return setupDatabase(t, db)
}

// tryConnection attempts to connect to PostgreSQL with given credentials
func tryConnection(host, port, user, password, database, sslmode string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, database, sslmode)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
}

// isDBNotExistError checks if the error is due to database not existing
func isDBNotExistError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "3D000")
}

// setupDatabase performs migration and cleanup for the test database
func setupDatabase(t *testing.T, db *gorm.DB) *gorm.DB {
	// Auto-migrate all models
	err := db.AutoMigrate(
		&models.PolicyMetadata{},
	)
	if err != nil {
		t.Skipf("Skipping test: could not migrate test database: %v", err)
		return nil
	}

	// Clean up test data before each test
	CleanupTestData(t, db)

	return db
}

// CleanupTestData removes all test data from the database
func CleanupTestData(t *testing.T, db *gorm.DB) {
	if db == nil {
		return
	}

	// Delete all policy metadata
	if err := db.Exec("DELETE FROM policy_metadata").Error; err != nil {
		t.Logf("Warning: could not cleanup policy_metadata: %v", err)
	}
}
