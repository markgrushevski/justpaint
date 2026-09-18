// Package migrate applies the embedded goose migrations at boot.
//
// Why the server does this itself, when goose is a perfectly good CLI: the
// deployment target has no shell. A free-tier host gives you environment
// variables and a container, and running a one-off command against the
// production database is a paid feature — so "remember to migrate before
// rolling the binary" is not a step that can be performed at all. The first
// live deploy proved the failure mode: the service came up, announced itself
// healthy, and logged `relation "matches" does not exist` four times every
// three seconds.
//
// The trade this accepts: a bad migration now takes the deploy down instead of
// being applied by hand under supervision. For a single-instance greenfield
// service that is the right side of the trade — an unmigrated database is
// broken anyway, and failing at boot is louder than failing per request.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	// Registers the "pgx" driver with database/sql, which is how goose reaches
	// Postgres without adding a second driver dependency.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/markgrushevski/justpaint/server/migrations"
)

// Run applies every OUTSTANDING migration and returns how many it applied.
//
// Already-applied versions are never re-run: goose records each one in its
// goose_db_version table and only executes what is missing from it, so a boot
// against a current schema costs one round trip and returns 0. That is what
// makes running this on every single start safe — the migration set is not
// replayed, it is reconciled.
//
// It opens its own short-lived database/sql handle rather than borrowing the
// pgx pool: goose speaks database/sql, and a migration connection wants none of
// the pool's tuning (lifetime recycling, connection ceiling) anyway.
func Run(ctx context.Context, dsn string, logger *slog.Logger) (int, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return 0, fmt.Errorf("migrate: open: %w", err)
	}
	defer func() { _ = db.Close() }()

	// One connection, and one migration run at a time across every instance: the
	// session-level advisory lock means a second boot waits for the first rather
	// than racing it through the same DDL. Single-instance today, but this is the
	// cheap half of making a second instance safe.
	db.SetMaxOpenConns(1)
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return 0, fmt.Errorf("migrate: locker: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS, goose.WithSessionLocker(locker))
	if err != nil {
		return 0, fmt.Errorf("migrate: provider: %w", err)
	}

	start := time.Now()
	results, err := provider.Up(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate: up: %w", err)
	}

	if len(results) == 0 {
		logger.Info("migrations: schema already current — nothing to apply")
		return 0, nil
	}
	for _, r := range results {
		logger.Info("migrations: applied", "version", r.Source.Version, "file", r.Source.Path, "duration_ms", r.Duration.Milliseconds())
	}
	logger.Info("migrations: done", "applied", len(results), "duration_ms", time.Since(start).Milliseconds())
	return len(results), nil
}
