package testutils

import (
	"fmt"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// SetupPostgresTestDB creates a PostgreSQL test database connection for integration tests.
// This connects to the same database that the service uses (via docker-compose).
//
// SECURITY NOTE: Database password must be provided via TEST_DB_PASSWORD environment variable.
// The function will fail if password is not set to prevent using weak default credentials.
func SetupPostgresTestDB(t *testing.T) *gorm.DB {
	// Use environment variables that match docker-compose.test.yml
	// These can be overridden for local testing
	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5432") // Default matches docker-compose port mapping
	user := getEnvOrDefault("TEST_DB_USERNAME", "postgres")

	// Require password to be explicitly set - no weak default
	password := os.Getenv("TEST_DB_PASSWORD")
	if password == "" {
		// Try the standard POSTGRES_PASSWORD env var as fallback
		password = os.Getenv("POSTGRES_PASSWORD")
		if password == "" {
			t.Fatalf("TEST_DB_PASSWORD or POSTGRES_PASSWORD environment variable must be set. " +
				"This prevents using weak default credentials.")
		}
	}

	database := getEnvOrDefault("TEST_DB_DATABASE", "policy_db")
	sslmode := getEnvOrDefault("TEST_DB_SSLMODE", "disable")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, database, sslmode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Skipf("Skipping database verification: could not connect to test database: %v", err)
		return nil
	}

	// Test connection
	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("Skipping database verification: failed to get sql.DB: %v", err)
		return nil
	}

	if err := sqlDB.Ping(); err != nil {
		t.Skipf("Skipping database verification: failed to ping database: %v", err)
		return nil
	}

	t.Logf("Connected to PostgreSQL test database: %s@%s:%s/%s", user, host, port, database)

	return db
}

// CleanupTestData removes all test data from the database
func CleanupTestData(t *testing.T, db *gorm.DB) {
	if db == nil {
		return
	}

	// Delete all field policies
	if err := db.Exec("DELETE FROM field_policies").Error; err != nil {
		t.Logf("Warning: could not cleanup field_policies: %v", err)
	}
}

// VerifyFieldPolicyExists checks if a field policy exists in the database
func VerifyFieldPolicyExists(t *testing.T, db *gorm.DB, schemaID, fieldName string) bool {
	if db == nil {
		t.Skip("Database connection not available for verification")
		return false
	}

	var count int64
	// Use raw SQL to avoid importing models package
	err := db.Table("field_policies").
		Where("schema_id = ? AND field_name = ?", schemaID, fieldName).
		Count(&count).Error

	if err != nil {
		t.Logf("Warning: could not verify field policy: %v", err)
		return false
	}

	return count > 0
}

// GetFieldPolicyCount returns the number of field policies
func GetFieldPolicyCount(t *testing.T, db *gorm.DB) int64 {
	if db == nil {
		return 0
	}

	var count int64
	// Use raw SQL to avoid importing models package
	if err := db.Table("field_policies").Count(&count).Error; err != nil {
		t.Logf("Warning: could not count field policies: %v", err)
		return 0
	}

	return count
}

// DBConfig holds database configuration for test setup
type DBConfig struct {
	Port     string
	Database string
	Password string
}

// WithTestDBEnv sets up environment variables for a specific test database and returns a cleanup function.
// This helper eliminates code duplication across test files.
//
// Example:
//
//	cleanup := testutils.WithTestDBEnv(t, testutils.DBConfig{
//	    Port:     "5433",
//	    Database: "policy_db",
//	    Password: "password",
//	})
//	defer cleanup()
func WithTestDBEnv(t *testing.T, config DBConfig) func() {
	originalPort := os.Getenv("TEST_DB_PORT")
	originalDB := os.Getenv("TEST_DB_DATABASE")
	originalPassword := os.Getenv("TEST_DB_PASSWORD")

	os.Setenv("TEST_DB_PORT", config.Port)
	os.Setenv("TEST_DB_DATABASE", config.Database)
	if config.Password != "" {
		if originalPassword == "" {
			os.Setenv("TEST_DB_PASSWORD", config.Password)
		}
	}

	return func() {
		if originalPort != "" {
			os.Setenv("TEST_DB_PORT", originalPort)
		} else {
			os.Unsetenv("TEST_DB_PORT")
		}
		if originalDB != "" {
			os.Setenv("TEST_DB_DATABASE", originalDB)
		} else {
			os.Unsetenv("TEST_DB_DATABASE")
		}
		if originalPassword == "" && config.Password != "" {
			os.Unsetenv("TEST_DB_PASSWORD")
		}
	}
}

// SetupConsentDB creates a PostgreSQL connection to the Consent Engine test database.
// This is a convenience wrapper around SetupPostgresTestDB with Consent Engine defaults.
// Returns nil if connection cannot be established (test will be skipped).
func SetupConsentDB(t *testing.T) *gorm.DB {
	cleanup := WithTestDBEnv(t, DBConfig{
		Port:     "5434",
		Database: "consent_db",
		Password: "password",
	})
	defer cleanup()

	return SetupPostgresTestDB(t)
}

// CleanupConsentRecord removes a consent record from the database.
// This is a helper function to reduce code duplication in consent tests.
func CleanupConsentRecord(t *testing.T, consentID string) {
	if consentID == "" {
		return
	}

	db := SetupConsentDB(t)
	if db == nil {
		t.Logf("Skipping cleanup for consent %s: database not available", consentID)
		return
	}

	if err := db.Exec("DELETE FROM consent_records WHERE consent_id = ?", consentID).Error; err != nil {
		t.Logf("Warning: failed to cleanup consent %s: %v", consentID, err)
	} else {
		t.Logf("Cleaned up consent: %s", consentID)
	}
}

// SetupPDPDB creates a PostgreSQL connection to the Policy Decision Point test database.
// This is a convenience wrapper around SetupPostgresTestDB with PDP defaults.
func SetupPDPDB(t *testing.T) *gorm.DB {
	cleanup := WithTestDBEnv(t, DBConfig{
		Port:     "5433",
		Database: "policy_db",
		Password: "password",
	})
	defer cleanup()

	return SetupPostgresTestDB(t)
}

// WithTestDBEnvFull sets up multiple environment variables for test database configuration.
// This is used for tests that need to override multiple DB connection parameters.
//
// Example:
//
//	cleanup := testutils.WithTestDBEnvFull(t, map[string]string{
//	    "TEST_DB_HOST": "invalid-host",
//	    "TEST_DB_PORT": "5432",
//	})
//	defer cleanup()
func WithTestDBEnvFull(t *testing.T, envVars map[string]string) func() {
	originals := make(map[string]string)
	for key := range envVars {
		originals[key] = os.Getenv(key)
		os.Setenv(key, envVars[key])
	}

	return func() {
		for key, originalValue := range originals {
			if originalValue != "" {
				os.Setenv(key, originalValue)
			} else {
				os.Unsetenv(key)
			}
		}
	}
}
