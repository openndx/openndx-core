-- Migration: Drop owner_email from consent_records
-- Date: 2026-07-17
-- Description: Removes the owner_email column and rebuilds the active-consent
--              uniqueness on (owner_id, app_id, status). In a DPI setup a user is
--              identified system-wide by a single canonical UID (owner_id) only;
--              email is no longer stored or used to key/authorize consents.
--
--              Run this against existing databases. Fresh databases created by
--              add_consent_unique_tuple_constraint.sql (or GORM AutoMigrate) already
--              have the correct shape and do not need this migration.
--
--              NOTE: If duplicate active consents exist for the same (owner_id, app_id)
--              that previously differed only by owner_email, the unique index below
--              will fail to create. Resolve those duplicates before running.

BEGIN;

-- Rebuild the partial unique index without owner_email
DROP INDEX IF EXISTS idx_consent_active_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_consent_active_unique
    ON consent_records(owner_id, app_id, status)
    WHERE status IN ('pending', 'approved');

-- Drop the owner_email secondary index and column
DROP INDEX IF EXISTS idx_consent_records_owner_email;
ALTER TABLE consent_records DROP COLUMN IF EXISTS owner_email;

-- Refresh table comment
COMMENT ON TABLE consent_records IS 'Stores consent records with partial unique constraint on active consents (pending/approved). Multiple historical records (rejected, expired, revoked) are allowed per (owner_id, app_id).';

COMMIT;

-- To rollback (re-add the column as nullable; original values are not recoverable):
-- ALTER TABLE consent_records ADD COLUMN IF NOT EXISTS owner_email VARCHAR(255);
-- CREATE INDEX IF NOT EXISTS idx_consent_records_owner_email ON consent_records(owner_email);