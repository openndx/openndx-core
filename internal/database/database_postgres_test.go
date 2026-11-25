// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Lanka Software Foundation

package database

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	coredb "github.com/OpenNSW/core/database"
	"github.com/stretchr/testify/require"
)

// TestPostgresInitializers verifies SQL and GORM use the same pool configuration
// and honor caller cancellation against a real PostgreSQL server.
func TestPostgresInitializers(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL initialization tests")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.NotNil(t, parsed.User)
	password, _ := parsed.User.Password()
	require.NotEmpty(t, parsed.Path, "TEST_DATABASE_URL must name a test database")
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	portNumber, err := strconv.Atoi(port)
	require.NoError(t, err)
	cfg := coredb.Config{
		Driver: coredb.Postgres,
		Postgres: &coredb.PostgresConfig{
			Host: parsed.Hostname(), Port: portNumber, User: parsed.User.Username(),
			Password: password, Name: parsed.Path[1:], SSLMode: parsed.Query().Get("sslmode"),
			Pool: coredb.PoolConfig{MaxOpenConns: 25, MaxIdleConns: 5, MaxConnLifetimeSeconds: 3600},
		},
	}
	for _, mode := range []string{"sql", "gorm"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var db *sql.DB
			if mode == "gorm" {
				wrapped, pool, err := OpenGORM(ctx, cfg, 20*time.Millisecond)
				require.NoError(t, err)
				db = pool
				t.Cleanup(func() { require.NoError(t, db.Close()) })
				underlying, err := wrapped.DB()
				require.NoError(t, err)
				require.Same(t, db, underlying)
				require.Equal(t, 1, db.Stats().OpenConnections, "GORM must reuse the initialized connection")
				require.NoError(t, wrapped.WithContext(ctx).Exec("SELECT 1").Error)
			} else {
				var err error
				db, err = Open(ctx, cfg, 20*time.Millisecond)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, db.Close()) })
				var value int
				require.NoError(t, db.QueryRowContext(ctx, "SELECT 1").Scan(&value))
				require.Equal(t, 1, value)
			}
			require.Equal(t, 25, db.Stats().MaxOpenConnections)
			require.NoError(t, db.PingContext(ctx), "the pool must outlive its initialization context")

			// Exercise the pool's actual idle cap and timeout, not only config fields.
			var release []func() error
			for range 6 {
				conn, err := db.Conn(ctx)
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.Close() })
				release = append(release, conn.Close)
			}
			for _, closeConn := range release {
				require.NoError(t, closeConn())
			}
			require.LessOrEqual(t, db.Stats().Idle, 5)
			require.Eventually(t, func() bool { return db.Stats().MaxIdleTimeClosed > 0 }, 3*time.Second, 20*time.Millisecond)
			require.NoError(t, db.Close())
			require.Error(t, db.PingContext(ctx))
		})
	}
	for _, tc := range []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		want    error
	}{
		{name: "canceled", context: func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, want: context.Canceled},
		{name: "expired deadline", context: func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, want: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.context()
			defer cancel()
			db, err := Open(ctx, cfg, time.Minute)
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, db)
			wrapped, db, err := OpenGORM(ctx, cfg, time.Minute)
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, wrapped)
			require.Nil(t, db)
		})
	}
}
