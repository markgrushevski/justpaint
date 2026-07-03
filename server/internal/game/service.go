// Package game implements the async drawing-duel lifecycle: create/auto-join a
// match, pin one shared prompt, and read redacted match state. Submit + judging
// (the authoritative render + Judge call) land in a later slice.
// See docs/GAME.md, docs/API.md §8.
package game

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
)

// GameCanvasSize is the canonical square game canvas (docs/GAME.md §2). Both
// duelists draw at exactly this size; it is echoed to the client so it configures
// the editor without guessing, and (later slice) enforced at submit.
const GameCanvasSize = 1080

// modeAsync is the only match mode in v1 (live WS is Phase 3 back-half).
const modeAsync = "async"

// Match statuses (docs/GAME.md §3). Only the states this slice reasons about are
// named here; the full enum lives on the DB check constraint.
const (
	statusOpen    = "open"
	statusDrawing = "drawing"
	statusDone    = "done"
)

// Sentinel errors the handler maps onto HTTP responses.
var (
	// ErrNoPrompts means no active prompt exists to pin — a server seeding fault,
	// not a client error (→ 500). The seed migration (00002) must have run.
	ErrNoPrompts = errors.New("game: no active prompts to pin")
	// ErrNotFound means the match is absent or the caller is not a player in it.
	// Hidden as 404 so match existence does not leak (docs/API.md §1, §8).
	ErrNotFound = errors.New("game: match not found")
)

// PlayerRow is one roster slot, decoupled from the generated row type so the
// redaction logic (buildMatchDTO) stays pure and table-testable.
type PlayerRow struct {
	UserID      string
	DisplayName *string
	DrawingID   *string // nil until the player submits (later slice)
}

// MatchView is the assembled match state the handler renders (applying the
// per-viewer visibility rules). It is the service's domain output, not a DTO.
type MatchView struct {
	ID         string
	Mode       string
	Status     string
	PromptID   string
	PromptText string
	Players    []PlayerRow
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Service holds the game business logic. It needs the pool (to run the
// create/join transaction) as well as the generated queries (for read paths).
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewService(pool *pgxpool.Pool, q *db.Queries) *Service {
	return &Service{pool: pool, q: q}
}

// CreateOrJoin is the single "play" entry point (docs/API.md §8 POST /api/matches,
// docs/DECISIONS.md "Matchmaking"). In one transaction it, in order:
//  1. auto-joins the oldest waiting async match the caller is not already in,
//     flipping it open→drawing (the roster is now full);
//  2. failing that, returns the caller's own still-open match if one exists, so a
//     waiting player tapping "play" again does not stack duplicate open matches;
//  3. failing that, creates a fresh open match with one random active prompt
//     pinned, and seats the caller as the first player.
func (s *Service) CreateOrJoin(ctx context.Context, userID string) (MatchView, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MatchView{}, fmt.Errorf("game: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // no-op once committed

	qtx := s.q.WithTx(tx)

	m, err := qtx.FindOpenMatchToJoin(ctx, userID)
	switch {
	case err == nil:
		// (1) A waiting match exists → seat the caller and start the round.
		if err := qtx.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: userID}); err != nil {
			return MatchView{}, fmt.Errorf("game: seat joiner: %w", err)
		}
		if m, err = qtx.UpdateMatchStatus(ctx, db.UpdateMatchStatusParams{ID: m.ID, Status: statusDrawing}); err != nil {
			return MatchView{}, fmt.Errorf("game: start match: %w", err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		// Nothing to join.
		if m, err = qtx.FindMyOpenMatch(ctx, userID); errors.Is(err, pgx.ErrNoRows) {
			// (3) No waiting match of mine either → create one.
			if m, err = s.createMatch(ctx, qtx, userID); err != nil {
				return MatchView{}, err // already wrapped / a sentinel
			}
		} else if err != nil {
			return MatchView{}, fmt.Errorf("game: find my open match: %w", err)
		}
		// (2) else: reuse my own open match, m already set.
	default:
		return MatchView{}, fmt.Errorf("game: find open match: %w", err)
	}

	view, err := s.assemble(ctx, qtx, m)
	if err != nil {
		return MatchView{}, err // already wrapped by assemble
	}
	if err := tx.Commit(ctx); err != nil {
		return MatchView{}, fmt.Errorf("game: commit tx: %w", err)
	}
	return view, nil
}

// createMatch pins one random active prompt and seats the creator. ErrNoPrompts
// surfaces when the prompt table has no active row (seed migration not run).
func (s *Service) createMatch(ctx context.Context, q *db.Queries, userID string) (db.Match, error) {
	prompt, err := q.PickRandomActivePrompt(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Match{}, ErrNoPrompts
		}
		return db.Match{}, fmt.Errorf("game: pick prompt: %w", err)
	}
	m, err := q.CreateMatch(ctx, prompt.ID)
	if err != nil {
		return db.Match{}, fmt.Errorf("game: create match: %w", err)
	}
	if err := q.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: userID}); err != nil {
		return db.Match{}, fmt.Errorf("game: seat creator: %w", err)
	}
	return m, nil
}

// Get returns match state for a caller who must be a player in it; a non-player
// (or a missing match) is hidden as ErrNotFound (→ 404, docs/API.md §8).
// Read-only, so it runs on the shared queries without a transaction.
func (s *Service) Get(ctx context.Context, userID, matchID string) (MatchView, error) {
	m, err := s.q.GetMatch(ctx, matchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MatchView{}, ErrNotFound
		}
		return MatchView{}, fmt.Errorf("game: get match: %w", err)
	}
	view, err := s.assemble(ctx, s.q, m)
	if err != nil {
		return MatchView{}, err // already wrapped by assemble
	}
	if !isPlayer(view.Players, userID) {
		return MatchView{}, ErrNotFound
	}
	return view, nil
}

// assemble loads the pinned prompt and roster for m and packs a MatchView. It
// runs on whatever queries handle it is given (a tx during create/join, the
// shared pool during a read) so the snapshot is consistent with its caller.
func (s *Service) assemble(ctx context.Context, q *db.Queries, m db.Match) (MatchView, error) {
	prompt, err := q.GetPromptByID(ctx, m.PromptID)
	if err != nil {
		return MatchView{}, fmt.Errorf("game: load prompt: %w", err)
	}
	rows, err := q.ListMatchPlayers(ctx, m.ID)
	if err != nil {
		return MatchView{}, fmt.Errorf("game: load roster: %w", err)
	}
	players := make([]PlayerRow, len(rows))
	for i, r := range rows {
		players[i] = PlayerRow{UserID: r.UserID, DisplayName: r.DisplayName, DrawingID: r.DrawingID}
	}
	return MatchView{
		ID:         m.ID,
		Mode:       m.Mode,
		Status:     m.Status,
		PromptID:   prompt.ID,
		PromptText: prompt.Text,
		Players:    players,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}, nil
}

func isPlayer(players []PlayerRow, userID string) bool {
	for _, p := range players {
		if p.UserID == userID {
			return true
		}
	}
	return false
}
