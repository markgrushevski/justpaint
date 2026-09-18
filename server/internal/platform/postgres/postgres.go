// Package postgres opens and verifies the application's Postgres connection pool.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates a pgx connection pool from a DSN and verifies it with a ping.
// The returned *pgxpool.Pool is concurrency-safe: create one at startup and
// share it across the app. The caller owns it and must Close() it on shutdown.
//
// maxConns bounds the pool explicitly. pgx's own default is max(4, NumCPU) —
// derived from the app host, blind to the database's max_connections — which is
// the wrong ceiling on a managed instance that allows only a handful.
func New(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}

	// Recycle connections so a long-running process picks up a DB failover and
	// doesn't accumulate stale server-side state. (pgx already defaults the rest:
	// HealthCheckPeriod=1m, MaxConnIdleTime=30m.)
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnLifetimeJitter = 5 * time.Minute
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: new pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return pool, nil
}
