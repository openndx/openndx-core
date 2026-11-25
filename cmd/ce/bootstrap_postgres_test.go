// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package main

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

// TestBootstrapConsentDatabase exercises the actual psql initialization script
// against a disposable PostgreSQL instance, including repeat runs and quoted names.
func TestBootstrapConsentDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL instance to test bootstrap")
	}
	psql, err := exec.LookPath("psql")
	require.NoError(t, err, "PostgreSQL bootstrap tests require psql")
	adminCfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	admin := stdlib.OpenDB(*adminCfg)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	script, err := os.ReadFile("../../init-db.sql")
	require.NoError(t, err)
	// Substitute the host paths for the schemas' container mount locations.
	bootstrap := string(script)
	for mount, file := range map[string]string{
		"/opt/openndx/ce-schema.sql":  "schema.sql",
		"/opt/openndx/pdp-schema.sql": "../pdp/schema.sql",
		"/opt/openndx/pb-schema.sql":  "../pb/schema.sql",
	} {
		schemaPath, err := filepath.Abs(file)
		require.NoError(t, err)
		bootstrap = strings.ReplaceAll(bootstrap, mount, schemaPath)
	}
	// The script also creates and fills the PDP and PB databases; drop those it creates.
	for _, name := range []string{"pdp", "portal_backend"} {
		var existed bool
		require.NoError(t, admin.QueryRow("SELECT EXISTS (SELECT FROM pg_database WHERE datname = $1)", name).Scan(&existed))
		if !existed {
			t.Cleanup(func() {
				_, err := admin.Exec("DROP DATABASE IF EXISTS " + pgx.Identifier{name}.Sanitize())
				require.NoError(t, err)
			})
		}
	}

	for _, tc := range []struct {
		name, value, want string
		provided          bool
	}{
		{name: "custom", provided: true, value: "ce_bootstrap_" + uuid.NewString()},
		{name: "quoted", provided: true, value: "ce_bootstrap_" + uuid.NewString() + " 'quoted\""},
		{name: "unset", want: "consent_engine"},
		{name: "empty", provided: true, want: "consent_engine"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			want := tc.want
			if want == "" {
				want = tc.value
			}
			var existed bool
			require.NoError(t, admin.QueryRowContext(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = $1)", want).Scan(&existed))
			if !existed {
				t.Cleanup(func() {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cleanupCancel()
					_, err := admin.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{want}.Sanitize())
					require.NoError(t, err)
				})
			}
			var defaultExisted bool
			require.NoError(t, admin.QueryRowContext(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = 'consent_engine')").Scan(&defaultExisted))
			var env []string
			for _, entry := range os.Environ() {
				// Drop DB_NAME_PB too, so the PB schema lands in portal_backend, which this test checks.
				if !strings.HasPrefix(entry, "DB_NAME_CE=") && !strings.HasPrefix(entry, "DB_NAME_PB=") {
					env = append(env, entry)
				}
			}
			if tc.provided {
				env = append(env, "DB_NAME_CE="+tc.value)
			}
			for range 2 {
				command := exec.CommandContext(ctx, psql, "--no-psqlrc", "--set", "ON_ERROR_STOP=1", "--dbname", dsn)
				command.Env = env
				command.Stdin = strings.NewReader(bootstrap)
				output, err := command.CombinedOutput()
				require.NoError(t, err, "%s", output)
			}
			targetCfg := adminCfg.Copy()
			targetCfg.Database = want
			target := stdlib.OpenDB(*targetCfg)
			t.Cleanup(func() { require.NoError(t, target.Close()) })
			var databaseName string
			var table, index sql.NullString
			require.NoError(t, target.QueryRowContext(ctx, `SELECT current_database(),
				to_regclass('public.consent_records')::text, to_regclass('public.idx_consent_active_unique')::text`).
				Scan(&databaseName, &table, &index))
			require.Equal(t, want, databaseName)
			require.True(t, table.Valid, "configured database must contain the consent table")
			require.True(t, index.Valid, "configured database must contain the active-consent index")
			if want != "consent_engine" {
				var defaultExists bool
				require.NoError(t, admin.QueryRowContext(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = 'consent_engine')").Scan(&defaultExists))
				require.Equal(t, defaultExisted, defaultExists, "custom bootstrap must not create the default CE database")
			}
		})
	}

	// The other services' databases get their schemas too.
	for database, table := range map[string]string{"pdp": "field_policies", "portal_backend": "members"} {
		targetCfg := adminCfg.Copy()
		targetCfg.Database = database
		target := stdlib.OpenDB(*targetCfg)
		var found sql.NullString
		err := target.QueryRow("SELECT to_regclass($1)::text", "public."+table).Scan(&found)
		require.NoError(t, target.Close())
		require.NoError(t, err)
		require.True(t, found.Valid, "%s database must contain the %s table", database, table)
	}
}
