// Package testdb starts one migrated Postgres container per test package and gives every test a clean database
package testdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"coursework/internal/store"
)

type Database struct {
	container *postgres.PostgresContainer
}

// Start runs the container, applies migrations and snapshots the clean state; always call Terminate
func Start(ctx context.Context) (*Database, error) {
	container, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("commitradar_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithSQLDriver("pgx"),
		postgres.BasicWaitStrategies(),
	)
	database := &Database{container: container}
	if err != nil {
		return database, err
	}

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return database, err
	}
	pool, err := store.Open(ctx, databaseURL)
	if err != nil {
		return database, err
	}
	err = store.Migrate(ctx, pool)
	pool.Close()
	if err != nil {
		return database, err
	}
	return database, container.Snapshot(ctx)
}

func (d *Database) Terminate() {
	_ = testcontainers.TerminateContainer(d.container)
}

// NewPool restores the snapshot and returns a pool that is closed when the test ends
func (d *Database) NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, d.container.Restore(ctx))

	databaseURL, err := d.container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := store.Open(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}
