package v1

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gov-dx-sandbox/portal-backend/v1/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseConfig holds GORM database connection configuration
type DatabaseConfig struct {
	Host            string
	Port            string
	Username        string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// NewDatabaseConfig creates a new GORM database configuration for V1
func NewDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Host:            getEnvOrDefault("DB_HOST", getEnvOrDefault("CHOREO_OPENDIF_DB_HOSTNAME", "localhost")),
		Port:            getEnvOrDefault("DB_PORT", getEnvOrDefault("CHOREO_OPENDIF_DB_PORT", "5432")),
		Username:        getEnvOrDefault("DB_USERNAME", getEnvOrDefault("CHOREO_OPENDIF_DB_USERNAME", "postgres")),
		Password:        getEnvOrDefault("DB_PASSWORD", getEnvOrDefault("CHOREO_OPENDIF_DB_PASSWORD", "password")),
		Database:        getEnvOrDefault("DB_NAME", getEnvOrDefault("CHOREO_OPENDIF_DB_DATABASENAME", "testdb2")),
		SSLMode:         getEnvOrDefault("DB_SSLMODE", "require"),
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
	}
}

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ConnectGormDB establishes a GORM connection to PostgreSQL
func ConnectGormDB(config *DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.Username, config.Password, config.Database, config.SSLMode)

	// Configure GORM logger
	gormLogger := logger.Default.LogMode(logger.Warn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying sql.DB to configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	slog.Info("Successfully connected to PostgreSQL database with GORM (V1)",
		"host", config.Host,
		"port", config.Port,
		"database", config.Database)

	// Only run migration if environment variable is set
	if os.Getenv("RUN_MIGRATION") == "true" {
		slog.Info("Running GORM auto-migration for V1 models")
		err = db.AutoMigrate(
			&models.Member{},
			&models.Schema{},
			&models.SchemaSubmission{},
			&models.Application{},
			&models.ApplicationSubmission{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to run auto-migration: %w", err)
		}
		slog.Info("GORM auto-migration completed successfully")
	} else {
		slog.Info("Database connected (migration skipped)")
	}

	return db, nil
}
