#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Lanka Software Foundation

set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
	CREATE DATABASE policy_db;
	CREATE DATABASE consent_db;
	CREATE DATABASE audit_db;
	CREATE DATABASE portal_db;
EOSQL
