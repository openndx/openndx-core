-- PostgreSQL schema for the Policy Decision Point.
-- Apply to the PDP database before starting the service. Safe to re-run.

BEGIN;

-- Enum types. CREATE TYPE has no IF NOT EXISTS, so skip types that already exist.
DO $$ BEGIN
    CREATE TYPE source_enum AS ENUM ('primary', 'fallback');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE owner_enum AS ENUM ('citizen');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS policy_metadata (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schema_id VARCHAR(255) NOT NULL,
    field_name TEXT NOT NULL,
    display_name TEXT,
    description TEXT,
    source source_enum NOT NULL DEFAULT 'fallback',
    policy_class TEXT NOT NULL,
    policy_params JSONB NOT NULL DEFAULT '{}',
    allow_list JSONB NOT NULL DEFAULT '{}',
    owner owner_enum,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Upgrade from v0.5: replace access_control_type and is_owner with a policy
-- class that keeps each field's current decision. Skipped once migrated.
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'policy_metadata'
          AND column_name = 'access_control_type'
    ) THEN
        ALTER TABLE policy_metadata ADD COLUMN IF NOT EXISTS policy_class TEXT;
        ALTER TABLE policy_metadata ADD COLUMN IF NOT EXISTS policy_params JSONB NOT NULL DEFAULT '{}';
        UPDATE policy_metadata
        SET policy_class = CASE
            WHEN access_control_type = 'restricted' AND NOT is_owner THEN 'allowlist_consent'
            ELSE 'allowlist'
        END;
        ALTER TABLE policy_metadata ALTER COLUMN policy_class SET NOT NULL;
        ALTER TABLE policy_metadata DROP COLUMN access_control_type, DROP COLUMN is_owner;
    END IF;
END $$;

DROP TYPE IF EXISTS access_control_type_enum;

-- One policy record per field of a schema
CREATE UNIQUE INDEX IF NOT EXISTS idx_policy_metadata_schema_field
    ON policy_metadata(schema_id, field_name);

COMMIT;
