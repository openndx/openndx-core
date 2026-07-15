package database

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/OpenNDX/openndx-core/exchange/consent-engine/internal/config"
	"github.com/OpenNDX/openndx-core/exchange/consent-engine/v1/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config holds GORM database connection configuration
type Config struct {
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
	MaxRetries      int
}

// NewDatabaseConfig creates a new GORM database configuration for V1
func NewDatabaseConfig(cfg *config.DBConfigs) *Config {
	return &Config{
		Host:            cfg.Host,
		Port:            cfg.Port,
		Username:        cfg.Username,
		Password:        cfg.Password,
		Database:        cfg.Database,
		SSLMode:         cfg.SSLMode,
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
		MaxRetries:      5,
	}
}

// ConnectGormDB establishes a GORM connection to PostgreSQL
func ConnectGormDB(config *Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.Username, config.Password, config.Database, config.SSLMode)

	// Configure GORM logger with custom config to prevent logging sensitive data
	// ParameterizedQueries=true prevents logging SQL parameters that might contain passwords
	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true, // Prevent logging SQL parameters that might contain sensitive data
			Colorful:                  false,
		},
	)

	// Retry connection with exponential backoff
	maxRetries := config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}
	var db *gorm.DB
	var err error

	for i := 0; i < maxRetries; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: gormLogger,
		})
		if err == nil {
			break
		}

		if i < maxRetries-1 {
			waitTime := time.Second * time.Duration(1<<i) // Exponential backoff: 1s, 2s, 4s, etc.
			slog.Warn("Failed to connect to database, retrying...",
				"attempt", i+1,
				"maxRetries", maxRetries,
				"error", err,
				"waitTime", waitTime)
			time.Sleep(waitTime)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, err)
	}

	// Verify db is not nil before proceeding
	if db == nil {
		return nil, fmt.Errorf("database connection is nil after successful connection attempt")
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
			&models.ConsentRecord{},
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
