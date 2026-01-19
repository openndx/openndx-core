package v1

import (
	"os"
	"testing"
	"time"
)

// TestConstants contains test constants
const (
	TestHost     = "localhost"
	TestPort     = "5432"
	TestUsername = "postgres"
	TestPassword = "password"
	TestDatabase = "testdb"
	TestSSLMode  = "require"
)

// TestDatabaseConfig creates a test database configuration
func TestDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Host:            TestHost,
		Port:            TestPort,
		Username:        TestUsername,
		Password:        TestPassword,
		Database:        TestDatabase,
		SSLMode:         TestSSLMode,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
	}
}

// WithEnvVars sets environment variables for testing and returns a cleanup function
func WithEnvVars(t *testing.T, vars map[string]string) func() {
	original := make(map[string]string)
	for key, value := range vars {
		original[key] = os.Getenv(key)
		os.Setenv(key, value)
	}
	return func() {
		for key, origValue := range original {
			if origValue == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, origValue)
			}
		}
	}
}

// TestEnvVars returns standard environment variables for testing
func TestEnvVars() map[string]string {
	return map[string]string{
		"DB_HOST":     "test-host",
		"DB_PORT":     "5432",
		"DB_USERNAME": "test-user",
		"DB_PASSWORD": "test-pass",
		"DB_NAME":     "test-db",
		"DB_SSLMODE":  "disable",
	}
}

// TestEnvVarsStandard returns standard environment variables for testing
func TestEnvVarsStandard() map[string]string {
	return map[string]string{
		"DB_HOST":     "standard-host",
		"DB_PORT":     "5434",
		"DB_USERNAME": "standard-user",
		"DB_PASSWORD": "standard-password",
		"DB_NAME":     "standard-db",
		"DB_SSLMODE":  "prefer",
	}
}
