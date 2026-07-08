package drawings

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

// minimalDocJSON is the smallest valid v1 document (mirrors the document
// package's own minimal fixture).
const minimalDocJSON = `{"version":1,"width":10,"height":10,"background":null,"layers":[{"id":"l","name":"L","visible":true,"opacity":1,"strokes":[]}]}`

// TestNameRoundtrip_DB verifies the SQL side of the name rules against a real
// Postgres — the part unit tests cannot see: the create-side COALESCE default
// and the update-side COALESCE keep-on-absent (queries/drawings.sql). It needs
// a migrated database; without DATABASE_URL it skips (matching local dev,
// where the server itself requires the exported env — CLAUDE.md Commands).
func TestNameRoundtrip_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed name roundtrip")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}

	q := db.New(pool)
	svc := NewService(q)

	owner, err := q.CreateUser(ctx, db.CreateUserParams{
		Login:        fmt.Sprintf("drawings-name-test-%d", time.Now().UnixNano()),
		PasswordHash: "test-only-not-a-real-hash",
	})
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "delete from drawings where owner_id = $1", owner.ID)
		_, _ = pool.Exec(ctx, "delete from users where id = $1", owner.ID)
	})

	raw := []byte(minimalDocJSON)
	doc, err := document.ParseAndValidate(raw)
	if err != nil {
		t.Fatalf("fixture document rejected: %v", err)
	}

	// Create without a name → the DB default.
	unnamed, err := svc.Create(ctx, owner.ID, nil, doc, raw)
	if err != nil {
		t.Fatalf("create (no name): %v", err)
	}
	if unnamed.Name != "new art" {
		t.Errorf("create without name: name = %q, want %q", unnamed.Name, "new art")
	}

	// Create with a name → get roundtrip carries it.
	named, err := svc.Create(ctx, owner.ID, strptr("sunset study"), doc, raw)
	if err != nil {
		t.Fatalf("create (named): %v", err)
	}
	got, err := svc.Get(ctx, owner.ID, named.ID)
	if err != nil {
		t.Fatalf("get after create: %v", err)
	}
	if got.Name != "sunset study" {
		t.Errorf("create→get roundtrip: name = %q, want %q", got.Name, "sunset study")
	}

	// Update WITH a name → replaced.
	upd, err := svc.Update(ctx, owner.ID, named.ID, strptr("dawn study"), doc, raw)
	if err != nil {
		t.Fatalf("update (named): %v", err)
	}
	if upd.Name != "dawn study" {
		t.Errorf("update with name: name = %q, want %q", upd.Name, "dawn study")
	}

	// Update WITHOUT a name → the existing name is kept, not clobbered.
	kept, err := svc.Update(ctx, owner.ID, named.ID, nil, doc, raw)
	if err != nil {
		t.Fatalf("update (name absent): %v", err)
	}
	if kept.Name != "dawn study" {
		t.Errorf("update without name: name = %q, want kept %q", kept.Name, "dawn study")
	}
}
