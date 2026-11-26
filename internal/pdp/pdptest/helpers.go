// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package pdptest

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// StringPtr returns a pointer to the given string value.
func StringPtr(s string) *string {
	return &s
}

// SetupTestDB creates an in-memory SQLite database for unit testing.
// It creates the field_policies table with SQLite-compatible schema.
// SQLite doesn't support PostgreSQL-specific features like gen_random_uuid(), enums, jsonb.
// This is fast and doesn't require a real database connection. Tests against
// PostgreSQL belong in tests/integration.
func SetupTestDB(t *testing.T) *gorm.DB {
	// Enforce foreign keys so deleting a field deletes its access grants, as in PostgreSQL.
	db, err := gorm.Open(sqlite.Open("file::memory:?_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Create tables manually for SQLite compatibility
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS field_policies (
			id TEXT PRIMARY KEY,
			schema_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			display_name TEXT,
			description TEXT,
			source TEXT NOT NULL DEFAULT 'fallback',
			requires_allowlist BOOLEAN NOT NULL,
			requires_consent BOOLEAN NOT NULL,
			owner TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(schema_id, field_name)
		)
	`
	if err := db.Exec(createTableSQL).Error; err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	createGrantsSQL := `
		CREATE TABLE IF NOT EXISTS access_grants (
			id TEXT PRIMARY KEY,
			schema_id TEXT NOT NULL,
			field_name TEXT NOT NULL,
			application_id TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
			consent_bypass BOOLEAN NOT NULL DEFAULT false,
			legal_basis TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (schema_id, field_name)
				REFERENCES field_policies(schema_id, field_name) ON DELETE CASCADE,
			CHECK (NOT consent_bypass OR legal_basis IS NOT NULL)
		)
	`
	if err := db.Exec(createGrantsSQL).Error; err != nil {
		t.Fatalf("Failed to create access_grants table: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_access_grants_active
		ON access_grants(schema_id, field_name, application_id, consent_bypass) WHERE status = 'active'`).Error; err != nil {
		t.Fatalf("Failed to create access_grants index: %v", err)
	}

	return db
}
