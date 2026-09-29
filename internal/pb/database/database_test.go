package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testParent struct {
	ID string `gorm:"primarykey"`
}

type testChild struct {
	ID       string     `gorm:"primarykey"`
	ParentID string     `gorm:"not null"`
	Parent   testParent `gorm:"foreignKey:ParentID;references:ID"`
}

func newSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// A single connection keeps every query on the same in-memory database.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

func TestAutoMigrate(t *testing.T) {
	t.Run("Migrates models in the given order", func(t *testing.T) {
		db := newSQLiteDB(t)

		err := AutoMigrate(db, &testParent{}, &testChild{})
		require.NoError(t, err)

		assert.True(t, db.Migrator().HasTable(&testParent{}))
		assert.True(t, db.Migrator().HasTable(&testChild{}))
	})

	t.Run("Returns error naming the failing model", func(t *testing.T) {
		db := newSQLiteDB(t)

		err := AutoMigrate(db, &testParent{}, "not-a-model")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to run auto-migration for string")
		assert.True(t, db.Migrator().HasTable(&testParent{}))
	})
}

func TestNewDatabaseConfig(t *testing.T) {
	// Isolate from ambient DB_* env so defaults are deterministic
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_USERNAME", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "")
	t.Setenv("DB_SSLMODE", "")

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
	t.Setenv("DB_HOST", "test-host")
	t.Setenv("DB_PORT", "5434")
	t.Setenv("DB_USERNAME", "test-user")
	t.Setenv("DB_PASSWORD", "test-pass")
	t.Setenv("DB_NAME", "test-db")
	t.Setenv("DB_SSLMODE", "disable")

	config := NewDatabaseConfig()
	assert.Equal(t, "test-host", config.Host)
	assert.Equal(t, "5434", config.Port)
	assert.Equal(t, "test-user", config.Username)
	assert.Equal(t, "test-pass", config.Password)
	assert.Equal(t, "test-db", config.Database)
	assert.Equal(t, "disable", config.SSLMode)
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("Returns env var when set", func(t *testing.T) {
		key := "TEST_ENV_VAR_12345"
		t.Setenv(key, "test-value")

		result := getEnvOrDefault(key, "default")
		assert.Equal(t, "test-value", result)
	})

	t.Run("Returns default when not set", func(t *testing.T) {
		key := "TEST_ENV_VAR_NONEXISTENT_12345"
		t.Setenv(key, "")

		result := getEnvOrDefault(key, "default-value")
		assert.Equal(t, "default-value", result)
	})

	t.Run("Returns default when empty string", func(t *testing.T) {
		key := "TEST_ENV_VAR_EMPTY_12345"
		t.Setenv(key, "")

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
