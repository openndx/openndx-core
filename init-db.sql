-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Lanka Software Foundation

-- Creates the databases required by the Exchange services and loads each
-- service's schema into its database.
-- Runs automatically on first Postgres container startup
-- (mounted into /docker-entrypoint-initdb.d).
--
-- The POSTGRES_USER (default: postgres) owns all databases.

-- Read the same CE and PB database names used by the services' Compose configuration.
\set ce_database consent_engine
\getenv ce_database DB_NAME_CE
SELECT COALESCE(NULLIF(:'ce_database', ''), 'consent_engine') AS ce_database \gset
\set pb_database portal_backend
\getenv pb_database DB_NAME_PB
SELECT COALESCE(NULLIF(:'pb_database', ''), 'portal_backend') AS pb_database \gset

-- Idempotent creation: skip databases that already exist so the script is
-- safe to re-run manually (the postgres image only auto-runs it on first init).
SELECT 'CREATE DATABASE pdp' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'pdp')\gexec
SELECT format('CREATE DATABASE %I', :'ce_database') WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'ce_database')\gexec
SELECT format('CREATE DATABASE %I', :'pb_database') WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'pb_database')\gexec

-- Bootstrap each service's schema from its single source file (mounted by compose.yml).
\connect :"ce_database"
\i /opt/openndx/ce-schema.sql
\connect pdp
\i /opt/openndx/pdp-schema.sql
\connect :"pb_database"
\i /opt/openndx/pb-schema.sql
