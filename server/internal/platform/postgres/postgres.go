// Package postgres opens and verifies the application's Postgres connection pool.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
		return nil, fmt.Errorf("postgres: ping: %w%s", err, connectHint(err))
	}

	return pool, nil
}

// ErrSchemaMissing means the database answered but has not been migrated, so the
// app can serve nothing useful. Callers distinguish it from an unreachable
// database because the two need opposite responses: wait and retry vs run goose.
var ErrSchemaMissing = errors.New("schema not applied")

// Ready reports whether the database is usable, not merely reachable.
//
// A plain Ping is not enough for a readiness probe: a fresh managed database
// accepts connections the moment it exists, so a deploy whose migrations were
// never run answers "ok" while every real request fails on a missing table.
// That is worse than a red probe — the platform reports the service healthy and
// the failure surfaces to users instead.
//
// The check is a catalog lookup (to_regclass), not a query against the table, so
// it costs nothing to run on every probe.
func Ready(ctx context.Context, pool *pgxpool.Pool, table string) error {
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w%s", err, connectHint(err))
	}

	var present bool
	if err := pool.QueryRow(ctx, "select to_regclass($1) is not null", table).Scan(&present); err != nil {
		return fmt.Errorf("schema check: %w", err)
	}
	if !present {
		return fmt.Errorf("%w: table %q is absent — apply the migrations (goose -dir server/migrations postgres \"$DATABASE_URL\" up)", ErrSchemaMissing, table)
	}
	return nil
}

// connectHint turns a dial failure that is really a deployment mismatch into
// something actionable, or returns "" when it has nothing to add.
//
// The case worth naming: managed providers increasingly publish their direct
// database host as IPv6-only (an IPv4 address is a paid add-on) while plenty of
// hosting platforms still egress IPv4-only. The result is `connect: network is
// unreachable` against a raw IPv6 literal — technically precise and useless
// unless you already know the shape of the problem. The fix is nearly always to
// use the provider's pooler endpoint, which is IPv4.
//
// Matching on the error text is crude, but this only ever decorates an error
// that has already failed; a miss costs nothing but the hint.
func connectHint(err error) string {
	msg := err.Error()
	if !strings.Contains(msg, "network is unreachable") && !strings.Contains(msg, "no route to host") {
		return ""
	}
	if !strings.Contains(msg, "[") { // no bracketed IPv6 literal in the dial error
		return ""
	}
	return "\n\thint: the database host resolved to IPv6 and this machine has no IPv6 route." +
		" Managed providers usually offer an IPv4 pooler endpoint — use that connection string instead" +
		" (on Supabase: Connect -> Session pooler, host *.pooler.supabase.com, user postgres.<project-ref>)."
}
