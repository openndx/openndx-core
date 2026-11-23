// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

// Package database initializes shared PostgreSQL pools for SQL and GORM callers.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	coredb "github.com/OpenNSW/core/database"
	_ "github.com/jackc/pgx/v5/stdlib" // Register PostgreSQL for OpenNSW's connection factory.
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open initializes and verifies a PostgreSQL pool using the supplied configuration.
// Connection setup is bounded by ten seconds or the caller's earlier deadline.
// The caller owns the returned pool and must close it on shutdown.
func Open(ctx context.Context, cfg coredb.Config, idleTimeout time.Duration) (*sql.DB, error) {
	if cfg.Driver != coredb.Postgres {
		return nil, fmt.Errorf("only PostgreSQL connections are supported")
	}
	if idleTimeout <= 0 {
		return nil, fmt.Errorf("database idle timeout must be positive")
	}
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	sqlDB, err := coredb.New(startupCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	sqlDB.SetConnMaxIdleTime(idleTimeout)
	return sqlDB, nil
}

// OpenGORM initializes one PostgreSQL pool and a GORM handle sharing that pool.
// The caller must close the returned SQL pool on shutdown. Initialization failures
// close the pool before returning; GORM does not open or ping a second connection.
func OpenGORM(ctx context.Context, cfg coredb.Config, idleTimeout time.Duration) (*gorm.DB, *sql.DB, error) {
	sqlDB, err := Open(ctx, cfg, idleTimeout)
	if err != nil {
		return nil, nil, err
	}
	gormDB, err := wrapGORM(sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	return gormDB, sqlDB, nil
}

// wrapGORM attaches GORM to an existing pool without opening or pinging another one.
func wrapGORM(sqlDB *sql.DB) (*gorm.DB, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("SQL database is required")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DisableAutomaticPing: true,
		// Translate driver errors (e.g. unique violations) into gorm.Err*
		// sentinels, so callers can map them without inspecting driver errors.
		TranslateError: true,
		// Log through the service's slog logger, so GORM's lines share its
		// format and fields.
		Logger: logger.NewSlogLogger(slog.Default(), logger.Config{
			SlowThreshold: 200 * time.Millisecond, LogLevel: logger.Warn,
			IgnoreRecordNotFoundError: true, ParameterizedQueries: true,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize GORM adapter: %w", err)
	}
	return db, nil
}
