-- Creates the databases required by the Exchange services.
-- Runs automatically on first Postgres container startup
-- (mounted into /docker-entrypoint-initdb.d).
--
-- The POSTGRES_USER (default: exchange) owns all databases.

-- Idempotent creation: skip databases that already exist so the script is
-- safe to re-run manually (the postgres image only auto-runs it on first init).
SELECT 'CREATE DATABASE pdp' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'pdp')\gexec
SELECT 'CREATE DATABASE consent_engine' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'consent_engine')\gexec
SELECT 'CREATE DATABASE orchestration_engine' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'orchestration_engine')\gexec
