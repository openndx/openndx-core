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
