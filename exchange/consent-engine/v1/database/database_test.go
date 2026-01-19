package database

import (
	"os"
	"testing"
	"time"

	"github.com/gov-dx-sandbox/exchange/consent-engine/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewDatabaseConfig(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USERNAME")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")
	os.Unsetenv("DB_SSLMODE")

	dbConfigs := &config.DBConfigs{
		Host:     "localhost",
		Port:     "5432",
		Username: "postgres",
		Password: "password",
		Database: "testdb",
		SSLMode:  "prefer",
	}
	config := NewDatabaseConfig(dbConfigs)

	assert.Equal(t, "localhost", config.Host)
	assert.Equal(t, "5432", config.Port)
	assert.Equal(t, "postgres", config.Username)
	assert.Equal(t, "password", config.Password)
	assert.Equal(t, "testdb", config.Database)
	assert.Equal(t, "prefer", config.SSLMode)
	assert.Equal(t, 25, config.MaxOpenConns)
	assert.Equal(t, 5, config.MaxIdleConns)
	assert.Equal(t, time.Hour, config.ConnMaxLifetime)
	assert.Equal(t, 30*time.Minute, config.ConnMaxIdleTime)
	assert.Equal(t, 5, config.MaxRetries)
}

func TestNewDatabaseConfig_WithEnvVars(t *testing.T) {
	os.Setenv("DB_HOST", "test-host")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("DB_USERNAME", "test-user")
	os.Setenv("DB_PASSWORD", "test-password")
	os.Setenv("DB_NAME", "test-db")
	os.Setenv("DB_SSLMODE", "require")
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USERNAME")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("DB_SSLMODE")
	}()

	dbConfigs := &config.DBConfigs{
		Host:     "test-host",
		Port:     "5433",
		Username: "test-user",
		Password: "test-password",
		Database: "test-db",
		SSLMode:  "require",
	}
	config := NewDatabaseConfig(dbConfigs)

	assert.Equal(t, "test-host", config.Host)
	assert.Equal(t, "5433", config.Port)
	assert.Equal(t, "test-user", config.Username)
	assert.Equal(t, "test-password", config.Password)
	assert.Equal(t, "test-db", config.Database)
	assert.Equal(t, "require", config.SSLMode)
}

// NOTE: Tests for ConnectGormDB with real database connections have been moved to
// tests/integration/database/database_test.go as integration tests.
// Unit tests should not use real database connections.

