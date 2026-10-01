package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// SchemaDB handles database operations for schemas
type SchemaDB struct {
	db *sql.DB
}

// NewSchemaDB creates a new schema database connection.
// On any failure after sql.Open succeeds, the connection pool is closed before returning.
func NewSchemaDB(ctx context.Context, connectionString string) (*SchemaDB, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	schemaDB := &SchemaDB{db: db}

	// Create tables if they don't exist
	if err := schemaDB.createTables(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return schemaDB, nil
}

// Close closes the database connection
func (s *SchemaDB) Close() error {
	return s.db.Close()
}

// createTables creates the necessary tables and enforces column/index constraints.
func (s *SchemaDB) createTables(ctx context.Context) error {
	// Create unified_schemas table
	createSchemasTable := `
	CREATE TABLE IF NOT EXISTS unified_schemas (
		id VARCHAR(36) PRIMARY KEY,
		version VARCHAR(50) UNIQUE NOT NULL,
		sdl TEXT NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'inactive',
		description TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		created_by VARCHAR(100) NOT NULL DEFAULT '',
		checksum VARCHAR(64) NOT NULL,
		is_active BOOLEAN NOT NULL DEFAULT FALSE
	);`

	if _, err := s.db.ExecContext(ctx, createSchemasTable); err != nil {
		return fmt.Errorf("failed to create unified_schemas table: %w", err)
	}

	// CREATE TABLE IF NOT EXISTS does not retrofit existing DBs — backfill then tighten.
	if err := s.ensureUnifiedSchemasConstraints(ctx); err != nil {
		return err
	}

	// Create schema_versions table for change tracking
	createVersionsTable := `
	CREATE TABLE IF NOT EXISTS schema_versions (
		id SERIAL PRIMARY KEY,
		from_version VARCHAR(50),
		to_version VARCHAR(50) NOT NULL,
		change_type VARCHAR(20) NOT NULL,
		changes JSONB,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		created_by VARCHAR(255) NOT NULL
	);`

	if _, err := s.db.ExecContext(ctx, createVersionsTable); err != nil {
		return fmt.Errorf("failed to create schema_versions table: %w", err)
	}

	return nil
}

// ensureUnifiedSchemasConstraints backfills NULLs, applies NOT NULL, and enforces
// at most one active schema. Safe to re-run on every startup.
func (s *SchemaDB) ensureUnifiedSchemasConstraints(ctx context.Context) error {
	backfill := `
		UPDATE unified_schemas SET
			description = COALESCE(description, ''),
			created_at = COALESCE(created_at, NOW()),
			updated_at = COALESCE(updated_at, NOW()),
			created_by = COALESCE(created_by, ''),
			is_active = COALESCE(is_active, FALSE)
		WHERE description IS NULL
		   OR created_at IS NULL
		   OR updated_at IS NULL
		   OR created_by IS NULL
		   OR is_active IS NULL`

	if _, err := s.db.ExecContext(ctx, backfill); err != nil {
		return fmt.Errorf("failed to backfill unified_schemas nulls: %w", err)
	}

	alters := []string{
		`ALTER TABLE unified_schemas ALTER COLUMN description SET DEFAULT ''`,
		`ALTER TABLE unified_schemas ALTER COLUMN description SET NOT NULL`,
		`ALTER TABLE unified_schemas ALTER COLUMN created_at SET DEFAULT NOW()`,
		`ALTER TABLE unified_schemas ALTER COLUMN created_at SET NOT NULL`,
		`ALTER TABLE unified_schemas ALTER COLUMN updated_at SET DEFAULT NOW()`,
		`ALTER TABLE unified_schemas ALTER COLUMN updated_at SET NOT NULL`,
		`ALTER TABLE unified_schemas ALTER COLUMN created_by SET DEFAULT ''`,
		`ALTER TABLE unified_schemas ALTER COLUMN created_by SET NOT NULL`,
		`ALTER TABLE unified_schemas ALTER COLUMN is_active SET DEFAULT FALSE`,
		`ALTER TABLE unified_schemas ALTER COLUMN is_active SET NOT NULL`,
	}
	for _, stmt := range alters {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to tighten unified_schemas columns: %w", err)
		}
	}

	// If multiple active rows already exist, keep the most recently updated one.
	dedupe := `
		WITH keeper AS (
			SELECT id FROM unified_schemas
			WHERE is_active = TRUE
			ORDER BY updated_at DESC NULLS LAST, created_at DESC NULLS LAST, id
			LIMIT 1
		)
		UPDATE unified_schemas
		SET is_active = FALSE
		WHERE is_active = TRUE
		  AND id NOT IN (SELECT id FROM keeper)`
	if _, err := s.db.ExecContext(ctx, dedupe); err != nil {
		return fmt.Errorf("failed to dedupe active unified_schemas: %w", err)
	}

	createOneActiveIndex := `
		CREATE UNIQUE INDEX IF NOT EXISTS unified_schemas_one_active
		ON unified_schemas ((1))
		WHERE is_active = TRUE`
	if _, err := s.db.ExecContext(ctx, createOneActiveIndex); err != nil {
		return fmt.Errorf("failed to create unified_schemas_one_active index: %w", err)
	}

	return nil
}

// Schema represents a database schema record
type Schema struct {
	ID          string    `json:"id" db:"id"`
	Version     string    `json:"version" db:"version"`
	SDL         string    `json:"sdl" db:"sdl"`
	Status      string    `json:"status" db:"status"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	CreatedBy   string    `json:"created_by" db:"created_by"`
	Checksum    string    `json:"checksum" db:"checksum"`
	IsActive    bool      `json:"is_active" db:"is_active"`
}

// CreateSchema creates a new schema in the database.
// New schemas are always stored inactive; activation goes through ActivateSchema.
func (s *SchemaDB) CreateSchema(ctx context.Context, schema *Schema) error {
	schema.IsActive = false
	if schema.Status == "" {
		schema.Status = "inactive"
	}

	query := `
		INSERT INTO unified_schemas (id, version, sdl, status, description, created_by, checksum, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE)`

	_, err := s.db.ExecContext(ctx, query, schema.ID, schema.Version, schema.SDL, schema.Status,
		schema.Description, schema.CreatedBy, schema.Checksum)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// GetSchemaByVersion retrieves a schema by version
func (s *SchemaDB) GetSchemaByVersion(ctx context.Context, version string) (*Schema, error) {
	query := `SELECT id, version, sdl, status, description, created_at, updated_at, created_by, checksum, is_active
			  FROM unified_schemas WHERE version = $1`

	row := s.db.QueryRowContext(ctx, query, version)

	schema := &Schema{}
	err := row.Scan(&schema.ID, &schema.Version, &schema.SDL, &schema.Status,
		&schema.Description, &schema.CreatedAt, &schema.UpdatedAt, &schema.CreatedBy,
		&schema.Checksum, &schema.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("schema version %s not found", version)
		}
		return nil, fmt.Errorf("failed to get schema: %w", err)
	}

	return schema, nil
}

// GetActiveSchema retrieves the currently active schema
func (s *SchemaDB) GetActiveSchema(ctx context.Context) (*Schema, error) {
	query := `SELECT id, version, sdl, status, description, created_at, updated_at, created_by, checksum, is_active
			  FROM unified_schemas WHERE is_active = TRUE LIMIT 1`

	row := s.db.QueryRowContext(ctx, query)

	schema := &Schema{}
	err := row.Scan(&schema.ID, &schema.Version, &schema.SDL, &schema.Status,
		&schema.Description, &schema.CreatedAt, &schema.UpdatedAt, &schema.CreatedBy,
		&schema.Checksum, &schema.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No active schema
		}
		return nil, fmt.Errorf("failed to get active schema: %w", err)
	}

	return schema, nil
}

// GetAllSchemas retrieves all schemas
func (s *SchemaDB) GetAllSchemas(ctx context.Context) ([]*Schema, error) {
	query := `SELECT id, version, sdl, status, description, created_at, updated_at, created_by, checksum, is_active
			  FROM unified_schemas ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get schemas: %w", err)
	}
	defer rows.Close()

	var schemas []*Schema
	for rows.Next() {
		schema := &Schema{}
		err := rows.Scan(&schema.ID, &schema.Version, &schema.SDL, &schema.Status,
			&schema.Description, &schema.CreatedAt, &schema.UpdatedAt, &schema.CreatedBy,
			&schema.Checksum, &schema.IsActive)
		if err != nil {
			return nil, fmt.Errorf("failed to scan schema: %w", err)
		}
		schemas = append(schemas, schema)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed while iterating schemas: %w", err)
	}

	return schemas, nil
}

// ActivateSchema activates a specific schema version
func (s *SchemaDB) ActivateSchema(ctx context.Context, version string) error {
	// Start transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Deactivate all schemas
	_, err = tx.ExecContext(ctx, "UPDATE unified_schemas SET is_active = FALSE")
	if err != nil {
		return fmt.Errorf("failed to deactivate schemas: %w", err)
	}

	// Activate the specified version
	result, err := tx.ExecContext(ctx, "UPDATE unified_schemas SET is_active = TRUE, updated_at = NOW() WHERE version = $1", version)
	if err != nil {
		return fmt.Errorf("failed to activate schema: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("schema version %s not found", version)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
