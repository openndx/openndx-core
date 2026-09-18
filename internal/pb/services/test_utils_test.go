package services

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("GetEnvOrDefault_WithValue", func(t *testing.T) {
		key := "TEST_ENV_VAR_12345"
		os.Setenv(key, "test-value")
		defer os.Unsetenv(key)

		result := getEnvOrDefault(key, "default")
		assert.Equal(t, "test-value", result)
	})

	t.Run("GetEnvOrDefault_WithoutValue", func(t *testing.T) {
		key := "TEST_ENV_VAR_NONEXISTENT_12345"
		os.Unsetenv(key)

		result := getEnvOrDefault(key, "default-value")
		assert.Equal(t, "default-value", result)
	})

	t.Run("GetEnvOrDefault_EmptyString", func(t *testing.T) {
		key := "TEST_ENV_VAR_EMPTY_12345"
		os.Setenv(key, "")
		defer os.Unsetenv(key)

		result := getEnvOrDefault(key, "default")
		assert.Equal(t, "default", result)
	})
}

// TestCleanupTestData is removed - it requires SQLite which should only be used in integration tests
// This test should be moved to integration tests if needed
