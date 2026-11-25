-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Lanka Software Foundation

-- PostgreSQL schema for Portal Backend.
-- Apply to the Portal Backend database before starting the service. Safe to re-run.

BEGIN;

CREATE TABLE IF NOT EXISTS members (
    member_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT,
    phone_number TEXT,
    -- The member's organization unit in the IdP
    ou_id TEXT NOT NULL CONSTRAINT uni_members_ou_id UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS schemas (
    schema_id TEXT PRIMARY KEY,
    member_id TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    sdl TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    version TEXT NOT NULL,
    schema_description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS applications (
    application_id TEXT PRIMARY KEY,
    application_name TEXT NOT NULL,
    application_description TEXT,
    selected_fields JSONB NOT NULL,
    member_id TEXT NOT NULL,
    version TEXT NOT NULL,
    idp_application_id TEXT,
    idp_client_id TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS schema_submissions (
    submission_id TEXT PRIMARY KEY,
    -- Set when the submission updates an existing schema
    previous_schema_id TEXT
        CONSTRAINT fk_schema_submissions_previous_schema REFERENCES schemas(schema_id),
    schema_name TEXT NOT NULL,
    schema_description TEXT,
    sdl TEXT NOT NULL,
    schema_endpoint TEXT NOT NULL,
    status TEXT NOT NULL,
    member_id TEXT NOT NULL,
    review TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS application_submissions (
    submission_id TEXT PRIMARY KEY,
    -- Set when the submission updates an existing application
    previous_application_id TEXT
        CONSTRAINT fk_application_submissions_previous_application REFERENCES applications(application_id),
    application_name TEXT NOT NULL,
    application_description TEXT,
    selected_fields JSONB NOT NULL,
    member_id TEXT NOT NULL,
    status TEXT NOT NULL,
    review TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

COMMIT;
