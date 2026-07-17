-- Migration: Create consent_records table with partial unique constraint for active consents
-- Date: 2025-12-12
-- Description: Creates the consent_records table with consent_id as primary key
--              and a partial unique index on (owner_id, app_id, status)
--              that only applies when status is 'pending' or 'approved'.
--              This ensures only one active consent exists per (owner_id, app_id),
--              while allowing multiple historical records (rejected, expired, revoked).
--              A user is identified system-wide by the single canonical owner_id (UID).

-- Create the consent_records table if it doesn't exist
CREATE TABLE IF NOT EXISTS consent_records (
    consent_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id VARCHAR(255) NOT NULL,
    app_id VARCHAR(255) NOT NULL,
    app_name VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    type VARCHAR(50) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    pending_expires_at TIMESTAMP WITH TIME ZONE,
    grant_expires_at TIMESTAMP WITH TIME ZONE,
    grant_duration VARCHAR(50) NOT NULL,
    fields JSONB NOT NULL,
    session_id VARCHAR(255),
    consent_portal_url TEXT NOT NULL,
    updated_by VARCHAR(255)
);

-- Create partial unique index for active consents (pending or approved)
-- status is intentionally kept only in the WHERE filter (not the key columns),
-- so at most one active consent (pending OR approved) can exist per (owner_id, app_id).
CREATE UNIQUE INDEX IF NOT EXISTS idx_consent_active_unique
    ON consent_records(owner_id, app_id)
    WHERE status IN ('pending', 'approved');

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_consent_records_owner_id ON consent_records(owner_id);
CREATE INDEX IF NOT EXISTS idx_consent_records_app_id ON consent_records(app_id);
CREATE INDEX IF NOT EXISTS idx_consent_records_status ON consent_records(status);
CREATE INDEX IF NOT EXISTS idx_consent_records_created_at ON consent_records(created_at);
CREATE INDEX IF NOT EXISTS idx_consent_records_pending_expires_at ON consent_records(pending_expires_at);
CREATE INDEX IF NOT EXISTS idx_consent_records_grant_expires_at ON consent_records(grant_expires_at);

-- Composite index for owner and app lookups
CREATE INDEX IF NOT EXISTS idx_consent_records_owner_app ON consent_records(owner_id, app_id);

-- Add comment to table
COMMENT ON TABLE consent_records IS 'Stores consent records with partial unique constraint on active consents (pending/approved). Multiple historical records (rejected, expired, revoked) are allowed per (owner_id, app_id).';

-- To rollback this migration (DROP TABLE):
-- DROP TABLE IF EXISTS consent_records CASCADE;