-- Create consent_records table
CREATE TABLE IF NOT EXISTS consent_records (
    consent_id uuid DEFAULT gen_random_uuid(),
    owner_id varchar(255) NOT NULL,
    app_id varchar(255) NOT NULL,
    app_name varchar(255),
    status varchar(50) NOT NULL,
    type varchar(50) NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    pending_expires_at timestamp with time zone,
    grant_expires_at timestamp with time zone,
    grant_duration varchar(50) NOT NULL,
    fields jsonb NOT NULL,
    session_id varchar(255),
    consent_portal_url text NOT NULL,
    updated_by varchar(255),
    PRIMARY KEY (consent_id)
);

-- Create indices
CREATE INDEX IF NOT EXISTS idx_consent_records_owner_id ON consent_records (owner_id);
CREATE INDEX IF NOT EXISTS idx_consent_records_app_id ON consent_records (app_id);
CREATE INDEX IF NOT EXISTS idx_consent_records_status ON consent_records (status);
CREATE INDEX IF NOT EXISTS idx_consent_records_created_at ON consent_records (created_at);
CREATE INDEX IF NOT EXISTS idx_consent_records_pending_expires_at ON consent_records (pending_expires_at);
CREATE INDEX IF NOT EXISTS idx_consent_records_grant_expires_at ON consent_records (grant_expires_at);
CREATE INDEX IF NOT EXISTS idx_consent_records_owner_app ON consent_records (owner_id, app_id);

-- Create partial unique index for active consents
-- status is kept only in the WHERE filter (not the key) so at most one active
-- consent (pending OR approved) can exist per (owner_id, app_id).
CREATE UNIQUE INDEX IF NOT EXISTS idx_consent_active_unique ON consent_records (owner_id, app_id) WHERE status IN ('pending', 'approved');
