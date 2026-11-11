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
    -- The application needs an unexpired access grant for the field
    requires_allowlist BOOLEAN NOT NULL,
    -- The owner must consent, unless the application holds a consent bypass grant
    requires_consent BOOLEAN NOT NULL,
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

-- Upgrade from policy classes: replace policy_class and policy_params with the
-- requires_allowlist and requires_consent flags, keeping each field's current
-- decision. An unrecognized class becomes the most restrictive policy. Skipped
-- once migrated.
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'policy_metadata'
          AND column_name = 'policy_class'
    ) THEN
        ALTER TABLE policy_metadata ADD COLUMN IF NOT EXISTS requires_allowlist BOOLEAN;
        ALTER TABLE policy_metadata ADD COLUMN IF NOT EXISTS requires_consent BOOLEAN;
        UPDATE policy_metadata
        SET requires_allowlist = policy_class <> ALL (ARRAY['open', 'consent']),
            requires_consent = policy_class <> ALL (ARRAY['open', 'allowlist']);
        ALTER TABLE policy_metadata
            ALTER COLUMN requires_allowlist SET NOT NULL,
            ALTER COLUMN requires_consent SET NOT NULL;
        ALTER TABLE policy_metadata DROP COLUMN policy_class, DROP COLUMN policy_params;
    END IF;
END $$;

-- One policy record per field of a schema
CREATE UNIQUE INDEX IF NOT EXISTS idx_policy_metadata_schema_field
    ON policy_metadata(schema_id, field_name);

-- One application's access to one field. Revoked grants are kept as a record.
-- Deleting a field's policy metadata deletes its grants.
--
-- A consent bypass grant lets the application read a field that asks for the
-- owner's consent without asking. It records the legal basis for it.
CREATE TABLE IF NOT EXISTS access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schema_id VARCHAR(255) NOT NULL,
    field_name TEXT NOT NULL,
    application_id TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'active'
        CONSTRAINT access_grants_status_check CHECK (status IN ('active', 'revoked')),
    consent_bypass BOOLEAN NOT NULL DEFAULT false,
    legal_basis TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT access_grants_field_fkey FOREIGN KEY (schema_id, field_name)
        REFERENCES policy_metadata(schema_id, field_name) ON DELETE CASCADE,
    -- A bypass grant always records its legal basis
    CONSTRAINT access_grants_bypass_check CHECK (NOT consent_bypass OR legal_basis IS NOT NULL)
);

-- At most one active plain grant and one active bypass grant per application
-- and field
CREATE UNIQUE INDEX IF NOT EXISTS idx_access_grants_active
    ON access_grants(schema_id, field_name, application_id, consent_bypass)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_access_grants_application
    ON access_grants(application_id, schema_id);

COMMIT;
