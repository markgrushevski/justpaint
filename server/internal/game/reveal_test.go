package game

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
)

// markedDocJSON is the smallest valid v1 document with a marker in its single
// layer's name, so a returned document can be traced to its owner.
func markedDocJSON(marker string) string {
	return `{"version":1,"width":10,"height":10,"background":null,` +
		`"layers":[{"id":"l","name":"` + marker + `","visible":true,"opacity":1,"strokes":[]}]}`
}

// TestPlayerDrawing_DB exercises the security gates of the opponent-canvas
// reveal (GET .../players/{userId}/drawing → Service.PlayerDrawing →
// GetMatchPlayerDrawing) against a real Postgres. The gates are folded into one
// SQL query (viewer-is-a-co-player, match-is-done, target-is-a-submitted-player),
// so a unit test can't see them; this is the proof that every miss is a hidden
// 404, including a non-member refused for a submitted target (the assertion that
// would catch a positional $2/$3 target↔viewer bind flip — docs/NOTES.md). Needs
// a migrated `DATABASE_URL`; skips otherwise.
//
// The handler's `uuid.Parse`→404 guard for malformed ids is not re-tested here
// (same pattern as Get/Result); the foreign-but-well-formed id → 404 path is
// covered below.
func TestPlayerDrawing_DB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB-backed reveal-gate test")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	// Close the pool via Cleanup, not defer: a test-body defer runs before the
	// t.Cleanup callbacks, so a deferred Close would shut the pool before the
	// row-cleanup below runs. Registered first, this runs last (Cleanup is LIFO).
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}

	q := db.New(pool)
	// PlayerDrawing only touches the queries; the render/judge/logger seams are
	// unused on this read path, so nil is safe.
	svc := NewService(pool, q, nil, nil, nil)

	// --- fixtures ---------------------------------------------------------
	// Cleanup is registered before any row is created (slices captured by
	// reference), so a mid-setup t.Fatalf still tears down whatever landed.
	var userIDs, matchIDs []string
	t.Cleanup(func() {
		// Scalar per-id deletes bind a uuid-string param reliably (the same
		// `where x = $1` form roundtrip_test uses); teardown errors are ignored.
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
	tags := map[string]string{} // user id → its document marker

	mkUser := func(tag string) string {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Login:        fmt.Sprintf("reveal-%s-%d", tag, ns),
			PasswordHash: "test-only-not-a-real-hash",
		})
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}
		userIDs = append(userIDs, u.ID)
		tags[u.ID] = "mark-" + tag
		return u.ID
	}

	// A prompt from the seeded set (migration 00002); a migrated DB has these.
	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		t.Fatalf("pick prompt (is the DB migrated + seeded? migration 00002): %v", err)
	}

	// mkMatch creates a match, seats the players, submits one marked drawing per
	// player, and sets the final status.
	mkMatch := func(status string, players ...string) string {
		m, err := q.CreateMatch(ctx, prompt.ID)
		if err != nil {
			t.Fatalf("create match: %v", err)
		}
		matchIDs = append(matchIDs, m.ID)
		for _, uid := range players {
			if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: uid}); err != nil {
				t.Fatalf("add player: %v", err)
			}
			raw := []byte(markedDocJSON(tags[uid]))
			doc, err := document.ParseAndValidate(raw)
			if err != nil {
				t.Fatalf("fixture doc rejected: %v", err)
			}
			d, err := q.CreateDrawing(ctx, db.CreateDrawingParams{
				OwnerID:    uid,
				MatchID:    &m.ID,
				DocVersion: int32(doc.Version),
				Width:      int32(doc.Width),
				Height:     int32(doc.Height),
				Document:   raw,
			})
			if err != nil {
				t.Fatalf("create drawing: %v", err)
			}
			if _, err := q.StampSubmission(ctx, db.StampSubmissionParams{
				MatchID: m.ID, UserID: uid, DrawingID: &d.ID,
			}); err != nil {
				t.Fatalf("stamp submission: %v", err)
			}
		}
		if _, err := q.UpdateMatchStatus(ctx, db.UpdateMatchStatusParams{ID: m.ID, Status: status}); err != nil {
			t.Fatalf("set match status: %v", err)
		}
		return m.ID
	}

	alice, bob := mkUser("alice"), mkUser("bob")
	carol, dave := mkUser("carol"), mkUser("dave")

	doneMatch := mkMatch(statusDone, alice, bob)     // both submitted → revealed
	liveMatch := mkMatch(statusDrawing, carol, dave) // both submitted, NOT done

	randomID := uuid.NewString()

	// --- gates ------------------------------------------------------------
	cases := []struct {
		name    string
		viewer  string
		match   string
		target  string
		wantErr bool // true ⇒ expect ErrNotFound (a hidden 404); false ⇒ expect the target's document
	}{
		{"co-player sees the opponent", alice, doneMatch, bob, false},
		{"co-player sees self (uniform participant read)", alice, doneMatch, alice, false},
		{"non-member refused for a submitted target (IDOR / role-flip)", carol, doneMatch, bob, true},
		{"opponent hidden until done", carol, liveMatch, dave, true},
		{"cross-match: target is a player of a DIFFERENT match", alice, doneMatch, carol, true},
		{"cross-match: viewer is not in this match", alice, liveMatch, dave, true},
		{"unknown (well-formed) match id", alice, randomID, bob, true},
		{"unknown (well-formed) target id", alice, doneMatch, randomID, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.PlayerDrawing(ctx, tc.viewer, tc.match, tc.target)
			if tc.wantErr {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("got err %v (doc=%s), want ErrNotFound (hidden 404)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// The returned document must be the TARGET's own (its marker), never the
			// viewer's — the reveal shows the opponent's canvas, not a copy of yours.
			want := []byte(tags[tc.target])
			if !bytes.Contains(got, want) {
				t.Errorf("returned document missing target marker %q; got %s", want, got)
			}
		})
	}
}
