package monitoring

import (
	"context"
	"database/sql"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// MonitorDBConnectionPool registers OTel metrics for a database connection pool.
// It creates observable gauges that periodically poll the sql.DBStats.
func MonitorDBConnectionPool(db *sql.DB) error {
	if db == nil {
		return errors.New("database connection is nil")
	}
	meter := otel.Meter("openndx")

	_, err := meter.Int64ObservableGauge(
		"db_connections_max",
		metric.WithDescription("The maximum number of open connections to the database"),
		metric.WithInt64Callback(func(_ context.Context, obs metric.Int64Observer) error {
			stats := db.Stats()
			obs.Observe(int64(stats.MaxOpenConnections))
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge(
		"db_connections_in_use",
		metric.WithDescription("The number of connections currently in use"),
		metric.WithInt64Callback(func(_ context.Context, obs metric.Int64Observer) error {
			stats := db.Stats()
			obs.Observe(int64(stats.InUse))
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge(
		"db_connections_idle",
		metric.WithDescription("The number of idle connections"),
		metric.WithInt64Callback(func(_ context.Context, obs metric.Int64Observer) error {
			stats := db.Stats()
			obs.Observe(int64(stats.Idle))
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableGauge(
		"db_connections_wait_count",
		metric.WithDescription("The total number of connections waited for"),
		metric.WithInt64Callback(func(_ context.Context, obs metric.Int64Observer) error {
			stats := db.Stats()
			obs.Observe(stats.WaitCount)
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Float64ObservableGauge(
		"db_connections_wait_duration_seconds_total",
		metric.WithDescription("The total time blocked waiting for a connection"),
		metric.WithFloat64Callback(func(_ context.Context, obs metric.Float64Observer) error {
			stats := db.Stats()
			obs.Observe(stats.WaitDuration.Seconds())
			return nil
		}),
	)
	return err
}
