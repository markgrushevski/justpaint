package game

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

// TestMatchStateJSON_PerViewerRedaction_DB is the DB-backed proof that the WS hub's
// per-viewer match_state (built via Service.MatchStateJSON) applies the SAME runtime
// redaction as REST: mid-round (status != done), a viewer sees their OWN drawingId and
// NEVER the opponent's (docs/GAME.md §4.2, docs/DESIGN-PHASE3-LIVE.md §3.6). This is the
// one assertion that would catch a hub that leaks the opponent's id before the match is
// done — the moment any membership-only reveal ships, that would be a live hole. Needs a
// migrated DATABASE_URL (docker compose up + goose up); skips otherwise, matching
// reveal_test.go / roundtrip_test.go.
func TestMatchStateJSON_PerViewerRedaction_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed per-viewer redaction test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	// Close LAST (registered first ⇒ LIFO ⇒ runs after the row cleanup below), so the
	// pool is still open when fixtures are torn down — the reveal_test.go pattern.
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}

	q := db.New(pool)
	// MatchStateJSON only touches the read queries; render/judge/logger/publisher seams
	// are unused, so nil is safe (NewService installs NopPublisher).
	svc := NewService(pool, q, nil, nil, nil)

	var userIDs, matchIDs []string
	t.Cleanup(func() {
		for _, mid := range matchIDs {
			_, _ = pool.Exec(ctx, "delete from match_players where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from drawings where match_id = $1", mid)
			_, _ = pool.Exec(ctx, "delete from matches where id = $1", mid)
		}
		for _, uid := range userIDs {
			_, _ = pool.Exec(ctx, "delete from users where id = $1", uid)
		}
	})

	ns := time.Now().UnixNano()
	mkUser := func(tag string) string {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Login:        fmt.Sprintf("wsredact-%s-%d", tag, ns),
			PasswordHash: "test-only-not-a-real-hash",
		})
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}
		userIDs = append(userIDs, u.ID)
		return u.ID
	}

	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}

	alice, bob := mkUser("alice"), mkUser("bob")

	m, err := q.CreateMatch(ctx, prompt.ID)
	if err != nil {
		t.Fatalf("create match: %v", err)
	}
	matchIDs = append(matchIDs, m.ID)

	// Seat + submit BOTH players (each gets a distinct drawing id), then force the match
	// to a mid-round `drawing` status: an artificial both-submitted-but-not-done state
	// that is exactly the redaction boundary we must hold on the wire.
	drawingID := map[string]string{}
	for _, uid := range []string{alice, bob} {
		if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
			t.Fatalf("add player: %v", err)
		}
		raw := []byte(markedDocJSON("mark-" + uid))
		doc, err := document.ParseAndValidate(raw)
		if err != nil {
			t.Fatalf("fixture doc rejected: %v", err)
		}
		d, err := q.CreateDrawing(ctx, db.CreateDrawingParams{
			OwnerID: uid, MatchID: &m.ID,
			DocVersion: int32(doc.Version), Width: int32(doc.Width), Height: int32(doc.Height),
			Document: raw,
		})
		if err != nil {
			t.Fatalf("create drawing: %v", err)
		}
		if _, err := q.StampSubmission(ctx, db.StampSubmissionParams{MatchID: m.ID, UserID: uid, DrawingID: &d.ID}); err != nil {
			t.Fatalf("stamp submission: %v", err)
		}
		drawingID[uid] = d.ID
	}
	if _, err := q.UpdateMatchStatus(ctx, db.UpdateMatchStatusParams{ID: m.ID, Status: statusDrawing}); err != nil {
		t.Fatalf("set match status drawing: %v", err)
	}

	// Alice's frame: her own drawingId present, bob's ABSENT (distinct UUIDs, so a
	// substring check is an unambiguous leak detector).
	aliceJSON, err := svc.MatchStateJSON(ctx, alice, m.ID)
	if err != nil {
		t.Fatalf("MatchStateJSON(alice): %v", err)
	}
	if !bytes.Contains(aliceJSON, []byte(drawingID[alice])) {
		t.Errorf("alice's match_state is missing her own drawingId: %s", aliceJSON)
	}
	if bytes.Contains(aliceJSON, []byte(drawingID[bob])) {
		t.Errorf("alice's match_state LEAKED bob's drawingId mid-round: %s", aliceJSON)
	}

	// Symmetric for bob.
	bobJSON, err := svc.MatchStateJSON(ctx, bob, m.ID)
	if err != nil {
		t.Fatalf("MatchStateJSON(bob): %v", err)
	}
	if !bytes.Contains(bobJSON, []byte(drawingID[bob])) {
		t.Errorf("bob's match_state is missing his own drawingId: %s", bobJSON)
	}
	if bytes.Contains(bobJSON, []byte(drawingID[alice])) {
		t.Errorf("bob's match_state LEAKED alice's drawingId mid-round: %s", bobJSON)
	}

	// A non-player never gets a frame — MatchStateJSON returns the hidden-404 sentinel.
	carol := mkUser("carol")
	if _, err := svc.MatchStateJSON(ctx, carol, m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("MatchStateJSON(non-player) = %v, want ErrNotFound", err)
	}
}
