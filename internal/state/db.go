package state

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect creates a PostgreSQL connection pool.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("state: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("state: ping: %w", err)
	}
	return pool, nil
}

// Close closes the connection pool.
func Close(pool *pgxpool.Pool) {
	if pool != nil {
		pool.Close()
	}
}

// HealthCheck verifies the database connection is alive.
func HealthCheck(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	return pool.QueryRow(ctx, "SELECT 1").Scan(&n)
}
