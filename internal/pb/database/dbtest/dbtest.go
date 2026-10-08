// Package dbtest provides database fixtures for Portal Backend tests. Import
// it only from _test.go files.
package dbtest

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SetupMockDB creates a mock database for testing using go-sqlmock
func SetupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	dialector := postgres.New(postgres.Config{
		Conn:       db,
		DriverName: "postgres",
	})

	gormDB, err := gorm.Open(dialector, &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return gormDB, mock, cleanup
}

// SetupSQLiteDB creates an in-memory SQLite database and migrates the given
// models into it.
func SetupSQLiteDB(t *testing.T, models ...any) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("Failed to connect to SQLite test database: %v", err)
	}

	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}
