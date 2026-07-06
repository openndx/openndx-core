package v1

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewDatabaseConfig(t *testing.T) {
	config := NewDatabaseConfig()
	assert.NotNil(t, config)
	assert.Equal(t, "localhost", config.Host)
	assert.Equal(t, "5432", config.Port)
	assert.Equal(t, "postgres", config.Username)
	assert.Equal(t, "password", config.Password)
	assert.Equal(t, "testdb2", config.Database)
	assert.Equal(t, "require", config.SSLMode)
	assert.Equal(t, 25, config.MaxOpenConns)
	assert.Equal(t, 5, config.MaxIdleConns)
	assert.Equal(t, time.Hour, config.ConnMaxLifetime)
	assert.Equal(t, 30*time.Minute, config.ConnMaxIdleTime)
}

func TestNewDatabaseConfig_WithEnvVars(t *testing.T) {
	// 1. Test fallback to CHOREO_* variables
	os.Setenv("CHOREO_OPENDIF_DB_HOSTNAME", "choreo-host")
	os.Setenv("CHOREO_OPENDIF_DB_PORT", "5433")
	os.Setenv("CHOREO_OPENDIF_DB_USERNAME", "choreo-user")
	os.Setenv("CHOREO_OPENDIF_DB_PASSWORD", "choreo-pass")
	os.Setenv("CHOREO_OPENDIF_DB_DATABASENAME", "choreo-db")
	os.Setenv("DB_SSLMODE", "disable")

	config := NewDatabaseConfig()
	assert.Equal(t, "choreo-host", config.Host)
	assert.Equal(t, "5433", config.Port)
	assert.Equal(t, "choreo-user", config.Username)
	assert.Equal(t, "choreo-pass", config.Password)
	assert.Equal(t, "choreo-db", config.Database)
	assert.Equal(t, "disable", config.SSLMode)

	// 2. Test standard DB_* variables taking precedence
	os.Setenv("DB_HOST", "test-host")
	os.Setenv("DB_PORT", "5434")
	os.Setenv("DB_USERNAME", "test-user")
	os.Setenv("DB_PASSWORD", "test-pass")
	os.Setenv("DB_NAME", "test-db")

	config = NewDatabaseConfig()
	assert.Equal(t, "test-host", config.Host)
	assert.Equal(t, "5434", config.Port)
	assert.Equal(t, "test-user", config.Username)
	assert.Equal(t, "test-pass", config.Password)
	assert.Equal(t, "test-db", config.Database)

	// Cleanup env vars
	os.Unsetenv("CHOREO_OPENDIF_DB_HOSTNAME")
	os.Unsetenv("CHOREO_OPENDIF_DB_PORT")
	os.Unsetenv("CHOREO_OPENDIF_DB_USERNAME")
	os.Unsetenv("CHOREO_OPENDIF_DB_PASSWORD")
	os.Unsetenv("CHOREO_OPENDIF_DB_DATABASENAME")
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USERNAME")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")
	os.Unsetenv("DB_SSLMODE")
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("Returns env var when set", func(t *testing.T) {
		key := "TEST_ENV_VAR_12345"
		os.Setenv(key, "test-value")
		defer os.Unsetenv(key)

		result := getEnvOrDefault(key, "default")
		assert.Equal(t, "test-value", result)
	})

	t.Run("Returns default when not set", func(t *testing.T) {
		key := "TEST_ENV_VAR_NONEXISTENT_12345"
		os.Unsetenv(key)

		result := getEnvOrDefault(key, "default-value")
		assert.Equal(t, "default-value", result)
	})

	t.Run("Returns default when empty string", func(t *testing.T) {
		key := "TEST_ENV_VAR_EMPTY_12345"
		os.Setenv(key, "")
		defer os.Unsetenv(key)

		result := getEnvOrDefault(key, "default")
		assert.Equal(t, "default", result)
	})
}

func TestConnectGormDB_InvalidConnection(t *testing.T) {
	config := &DatabaseConfig{
		Host:     "invalid-host",
		Port:     "5432",
		Username: "invalid-user",
		Password: "invalid-password",
		Database: "invalid-db",
		SSLMode:  "disable",
	}

	_, err := ConnectGormDB(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}
