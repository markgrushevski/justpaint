package migrate

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/markgrushevski/justpaint/server/migrations"
)

// TestRun_Idempotent_DB is the guarantee that makes migrating on every boot
// safe: an already-applied migration is never executed twice. goose records each
// applied version in goose_db_version and reconciles against it, so the second
// run must apply exactly zero — otherwise every restart would replay DDL that
// has already run (and, for a non-idempotent statement, fail).
//
// Skips without DATABASE_URL, like the other DB-backed tests; CI runs it for
// real against the postgres service.
func TestRun_Idempotent_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping the DB-backed migration test")
	}

	ctx := context.Background()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Whatever the database's starting state, this leaves it current.
	if _, err := Run(ctx, dsn, quiet); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	applied, err := Run(ctx, dsn, quiet)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if applied != 0 {
		t.Errorf("second Run applied %d migration(s); an up-to-date schema must apply none", applied)
	}

	// A third time, because "runs on every boot" means exactly this.
	applied, err = Run(ctx, dsn, quiet)
	if err != nil {
		t.Fatalf("third Run: %v", err)
	}
	if applied != 0 {
		t.Errorf("third Run applied %d migration(s), want 0", applied)
	}
}

// TestRun_RecordsEveryMigration_DB pins the other half: the version table ends
// up holding one row per migration file. A mismatch means goose and the embedded
// set disagree about what exists — the state where "nothing to apply" would be a
// lie rather than a fact.
func TestRun_RecordsEveryMigration_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping the DB-backed migration test")
	}

	ctx := context.Background()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := Run(ctx, dsn, quiet); err != nil {
		t.Fatalf("Run: %v", err)
	}

	files, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	wantVersions := 0
	for _, f := range files {
		if !f.IsDir() && len(f.Name()) > 4 && f.Name()[len(f.Name())-4:] == ".sql" {
			wantVersions++
		}
	}
	if wantVersions == 0 {
		t.Fatal("no .sql files are embedded — go:embed picked up nothing")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Version 0 is goose's own baseline row, not one of ours.
	var recorded int
	if err := db.QueryRowContext(ctx, "select count(*) from goose_db_version where version_id > 0").Scan(&recorded); err != nil {
		t.Fatalf("count applied versions: %v", err)
	}
	if recorded != wantVersions {
		t.Errorf("goose_db_version holds %d applied version(s), embedded set has %d files", recorded, wantVersions)
	}
}
