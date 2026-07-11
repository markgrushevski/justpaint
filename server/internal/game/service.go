// Package game implements the async drawing-duel lifecycle: create/auto-join a
// match, pin one shared prompt, draw the same prompt, submit, judge out-of-band,
// and reveal the result — the full open → drawing → judging → done loop. The
// render and judge are seams (internal/render, internal/judge); only the
// pixel-authoritative Node render worker is still stubbed. See docs/GAME.md,
// docs/API.md §8.
package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
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
	statusOpen      = "open"
	statusDrawing   = "drawing"
	statusJudging   = "judging"
	statusDone      = "done"
	statusAbandoned = "abandoned"
)

// Match resolutions (docs/DESIGN-PHASE3-LIVE.md §2.7): how a `done` match was
// decided. Only `done` rows carry one; `abandoned` (no result) stays null.
const (
	resolutionJudged  = "judged"
	resolutionForfeit = "forfeit"
)

// roundSeconds is the drawing-round length, stamped as the absolute
// drawing_deadline (now() + roundSeconds) when the roster fills and the match
// flips open→drawing. A Go constant, not a column — changing it needs no
// migration (docs/DESIGN-PHASE3-LIVE.md §2.1).
const roundSeconds = 90

// Sentinel errors the handler maps onto HTTP responses.
var (
	// ErrNoPrompts means no active prompt exists to pin — a server seeding fault,
	// not a client error (→ 500). The seed migration (00002) must have run.
	ErrNoPrompts = errors.New("game: no active prompts to pin")
	// ErrNotFound means the match is absent or the caller is not a player in it.
	// Hidden as 404 so match existence does not leak (docs/API.md §1, §8).
	ErrNotFound = errors.New("game: match not found")
	// ErrNotPlayer: the caller is authenticated but not a player in this match. On
	// submit — a known-ownership violation the client already knows exists — this
	// maps to 403 (docs/API.md §1, §8.3), distinct from the read path's 404 hide.
	ErrNotPlayer = errors.New("game: not a player in this match")
	// ErrNotSubmittable: the match is not in the drawing state (already judging/
	// done/abandoned) → 409.
	ErrNotSubmittable = errors.New("game: match not accepting submissions")
	// ErrAlreadySubmitted: this player already submitted → 409 (no double-submit).
	ErrAlreadySubmitted = errors.New("game: already submitted")
	// ErrRoundExpired: the drawing deadline passed before this submit landed. The
	// late submission is NOT stamped; the match is resolved (forfeit/abandoned)
	// instead → 409 (docs/DESIGN-PHASE3-LIVE.md §2.4).
	ErrRoundExpired = errors.New("game: round deadline passed")
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
	// DrawingDeadline is the absolute round deadline (DB clock), nil while `open`.
	DrawingDeadline *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Service holds the game business logic. It needs the pool (to run the
// create/join + submit transactions), the generated queries (read paths), and
// the two seams — the render worker and the judge — both behind interfaces so the
// stub/fake swap for real impls with no loop change. The logger is for the
// out-of-band judging goroutine, whose failures have no request to return to.
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	renderer render.Renderer
	judge    judge.Judge
	logger   *slog.Logger
	// publisher pushes committed transitions to the realtime layer. It defaults to
	// NopPublisher (no realtime) and is swapped for the ws hub via SetPublisher, so
	// NewService keeps its signature and the round-deadline suite runs unchanged
	// (docs/DESIGN-PHASE3-LIVE.md §3.2).
	publisher Publisher
}

func NewService(pool *pgxpool.Pool, q *db.Queries, renderer render.Renderer, jdg judge.Judge, logger *slog.Logger) *Service {
	return &Service{pool: pool, q: q, renderer: renderer, judge: jdg, logger: logger, publisher: NopPublisher{}}
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

	joined := false // true only on the join branch, which publishes match_state post-commit
	m, err := qtx.FindOpenMatchToJoin(ctx, userID)
	switch {
	case err == nil:
		// (1) A waiting match exists → seat the caller and start the round.
		if err := qtx.AddMatchPlayer(ctx, db.AddMatchPlayerParams{MatchID: m.ID, UserID: userID}); err != nil {
			return MatchView{}, fmt.Errorf("game: seat joiner: %w", err)
		}
		// Roster full → start the round AND stamp the server-authoritative deadline
		// (now() + roundSeconds, the DB's own clock). Replaces the generic status
		// flip only at this open→drawing site (docs/DESIGN-PHASE3-LIVE.md §2.3).
		if m, err = qtx.SetMatchDrawing(ctx, db.SetMatchDrawingParams{ID: m.ID, RoundSeconds: roundSeconds}); err != nil {
			return MatchView{}, fmt.Errorf("game: start match: %w", err)
		}
		joined = true
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
	// Post-commit: the waiting player's socket learns the opponent joined and the
	// round started (open→drawing stamped the deadline). Only the join branch flips
	// state; the reuse/create branches leave a lone-open match nobody is watching yet
	// (docs/DESIGN-PHASE3-LIVE.md §2.4 publish sites).
	if joined {
		s.publisher.MatchChanged(m.ID)
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
		ID:              m.ID,
		Mode:            m.Mode,
		Status:          m.Status,
		PromptID:        prompt.ID,
		PromptText:      prompt.Text,
		Players:         players,
		DrawingDeadline: m.DrawingDeadline,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
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

// SubmitResult is the small post-submit state the handler echoes (docs/API.md
// §8.3): the match status now (drawing while awaiting the opponent, judging once
// both are in) and the caller's stored drawing id.
type SubmitResult struct {
	Status    string
	DrawingID string
	// Deadline is the round's absolute drawing deadline, echoed so the client can
	// re-anchor its countdown against the server clock after a submit.
	Deadline *time.Time
}

// Submit persists the caller's drawing for the match, stamps their roster slot,
// and — when it is the last outstanding submission — flips the match to judging
// and scores it out-of-band (docs/GAME.md §4.1, docs/API.md §8.3). The document is
// already validated + canvas-checked by the handler; raw is the canonical jsonb.
//
// Errors: ErrNotFound (no such match → 404), ErrNotPlayer (403), ErrNotSubmittable
// (match not in drawing → 409), ErrAlreadySubmitted (double-submit → 409).
func (s *Service) Submit(ctx context.Context, userID, matchID string, doc document.Document, raw []byte) (SubmitResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("game: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	// Lock the match row so two simultaneous final submits can't both miss "I'm
	// last" — the flip to judging must be computed on a stable roster.
	m, err := qtx.GetMatchForUpdate(ctx, matchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmitResult{}, ErrNotFound
		}
		return SubmitResult{}, fmt.Errorf("game: lock match: %w", err)
	}

	// Must be a player. Unlike the read path (hidden 404), a submit to a match you
	// are not in is a known-ownership violation → ErrNotPlayer → 403 (API.md §8.3).
	player, err := qtx.GetMatchPlayer(ctx, db.GetMatchPlayerParams{MatchID: matchID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmitResult{}, ErrNotPlayer
		}
		return SubmitResult{}, fmt.Errorf("game: get player: %w", err)
	}

	// Defense-in-depth deadline enforcement on the DB clock (server_now, captured
	// under this lock). A submit landing after the deadline but before the next
	// sweep tick still sees status='drawing' and would otherwise be stamped —
	// silently converting the opponent's forfeit win into a judged match. Resolve
	// the expiry here instead; the late submission is NOT stamped (we return before
	// CreateDrawing/StampSubmission). Fire judging only for the defensive
	// both-submitted case (docs/DESIGN-PHASE3-LIVE.md §2.4).
	if isExpiredDrawing(m) {
		outcome, err := s.resolveExpiry(ctx, qtx, m)
		if err != nil {
			return SubmitResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SubmitResult{}, fmt.Errorf("game: commit tx: %w", err)
		}
		if outcome == outcomeJudging {
			go s.judgeMatch(matchID)
		}
		// Uniform post-commit tail, identical to the sweeper's: the WINNING opponent
		// (not on this request) is notified the instant this late submit forfeited the
		// round (docs/DESIGN-PHASE3-LIVE.md §2.4, §3.2).
		s.publishOutcome(matchID, outcome)
		return SubmitResult{}, ErrRoundExpired
	}

	if m.Status != statusDrawing {
		return SubmitResult{}, ErrNotSubmittable
	}
	if player.SubmittedAt != nil {
		return SubmitResult{}, ErrAlreadySubmitted
	}

	// Persist the submission as a match-linked drawing (owner = caller). The server
	// derives doc_version/width/height from the validated document.
	d, err := qtx.CreateDrawing(ctx, db.CreateDrawingParams{
		OwnerID:    userID,
		MatchID:    &matchID,
		Name:       nil, // duel submissions have no user-facing name; the SQL default 'new art' applies
		DocVersion: int32(doc.Version),
		Width:      int32(doc.Width),
		Height:     int32(doc.Height),
		Document:   raw,
	})
	if err != nil {
		return SubmitResult{}, fmt.Errorf("game: create submission: %w", err)
	}

	// Stamp the slot; the `submitted_at is null` guard makes a racing double-tap a
	// no-op (stamped == 0 ⇒ already submitted).
	stamped, err := qtx.StampSubmission(ctx, db.StampSubmissionParams{MatchID: matchID, UserID: userID, DrawingID: &d.ID})
	if err != nil {
		return SubmitResult{}, fmt.Errorf("game: stamp submission: %w", err)
	}
	if stamped == 0 {
		return SubmitResult{}, ErrAlreadySubmitted
	}

	remaining, err := qtx.CountUnsubmitted(ctx, matchID)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("game: count unsubmitted: %w", err)
	}
	status := statusDrawing
	triggerJudging := false
	if remaining == 0 {
		// SetMatchJudging (not the generic status flip) stamps judging_started_at +
		// the attempt counter, so the stuck-judging watchdog measures staleness per
		// attempt (docs/DESIGN-PHASE3-LIVE.md §2.3, §2.6).
		if _, err = qtx.SetMatchJudging(ctx, matchID); err != nil {
			return SubmitResult{}, fmt.Errorf("game: to judging: %w", err)
		}
		status = statusJudging
		triggerJudging = true
	}

	if err := tx.Commit(ctx); err != nil {
		return SubmitResult{}, fmt.Errorf("game: commit tx: %w", err)
	}

	// Post-commit realtime: tell the room this player submitted (the frame carries
	// {userId}; clients ignore their own), and — if this was the last submission that
	// flipped the match to judging — that judging began (docs/DESIGN-PHASE3-LIVE.md §3.2).
	s.publisher.PlayerSubmitted(matchID, userID)
	if triggerJudging {
		s.publisher.Judging(matchID)
		// Out-of-band: the submit response returns immediately (202); the verdict is
		// produced by the async pass (docs/API.md §8.3). In-process for v1 — a crash
		// mid-judge leaves the match in judging (no auto-retry; see docs/NOTES.md).
		go s.judgeMatch(matchID)
	}
	return SubmitResult{Status: status, DrawingID: d.ID, Deadline: m.DrawingDeadline}, nil
}

// judgeMatch runs the judging pass for a match that just entered judging, on its
// own background context (the request that triggered it has already returned).
func (s *Service) judgeMatch(matchID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.runJudging(ctx, matchID); err != nil {
		// v1: log and leave the match in judging (no auto-retry). A restart-time
		// sweeper lands with the real render worker (docs/NOTES.md, docs/IDEAS.md).
		s.logger.Error("judge match", "matchID", matchID, "err", err)
	}
}

// runJudging renders both submissions to the authoritative raster, scores them,
// maps the positional winner onto a player id, applies Elo, and flips the match
// to done. Idempotent: a match not in judging is a no-op.
func (s *Service) runJudging(ctx context.Context, matchID string) error {
	m, err := s.q.GetMatch(ctx, matchID)
	if err != nil {
		return fmt.Errorf("game: get match: %w", err)
	}
	if m.Status != statusJudging {
		return nil // already judged / not ready — nothing to do
	}
	prompt, err := s.q.GetPromptByID(ctx, m.PromptID)
	if err != nil {
		return fmt.Errorf("game: get prompt: %w", err)
	}
	subs, err := s.q.GetSubmissionsForJudging(ctx, matchID)
	if err != nil {
		return fmt.Errorf("game: get submissions: %w", err)
	}
	if len(subs) != 2 {
		return fmt.Errorf("game: expected 2 submissions, got %d", len(subs))
	}

	// subs[0] = image A, subs[1] = image B — the stable order (GAME.md §7.1).
	imgA, err := s.renderSubmission(ctx, subs[0].Document)
	if err != nil {
		return fmt.Errorf("game: render A: %w", err)
	}
	imgB, err := s.renderSubmission(ctx, subs[1].Document)
	if err != nil {
		return fmt.Errorf("game: render B: %w", err)
	}

	res, err := s.judge.Score(ctx, judge.Request{Prompt: prompt.Text, ImageA: imgA, ImageB: imgB})
	if err != nil {
		return fmt.Errorf("game: judge: %w", err)
	}
	if err := res.Validate(); err != nil {
		return fmt.Errorf("game: judge result: %w", err)
	}

	// Map positional winner → concrete player id (null on tie), and A's Elo score.
	var winner *string
	sa := scoreTie
	switch res.Winner {
	case judge.WinnerA:
		winner = &subs[0].UserID
		sa = scoreWin
	case judge.WinnerB:
		winner = &subs[1].UserID
		sa = scoreLoss
	case judge.WinnerTie:
		// winner stays nil, sa stays scoreTie — the initialized defaults.
	}

	ratingA, ratingB := int(subs[0].Rating), int(subs[1].Rating)
	afterA, afterB := computeElo(ratingA, ratingB, sa)

	return s.persistResult(ctx, matchID, res, winner,
		playerResult{userID: subs[0].UserID, score: &res.ScoreA, before: ratingA, after: afterA},
		playerResult{userID: subs[1].UserID, score: &res.ScoreB, before: ratingB, after: afterB},
	)
}

// renderSubmission re-parses a stored document (validated at submit; a failure
// here is corruption) and renders the authoritative judged raster.
func (s *Service) renderSubmission(ctx context.Context, raw []byte) ([]byte, error) {
	doc, err := document.ParseAndValidate(raw)
	if err != nil {
		return nil, fmt.Errorf("game: reparse submission: %w", err)
	}
	return s.renderer.Render(ctx, doc)
}

// playerResult carries one player's terminal record into writeFinalResult: the
// judge similarity score (nil on a forfeit — no judge ran), the Elo snapshot around
// the match, all keyed by user_id so a seat mix-up can't write to the wrong row.
type playerResult struct {
	userID string
	score  *float64
	before int
	after  int
}

// finalResult is the seat-independent terminal state both the judged path
// (persistResult) and the forfeit path (resolveExpiry) hand to writeFinalResult.
// winner is the winner's user_id, or nil on a tie (judged only). Players are
// identified by user_id inside `players`, never by seat index, so Elo can never be
// applied to the wrong seat. resolution is resolutionJudged | resolutionForfeit.
type finalResult struct {
	winner     *string
	players    []playerResult
	reason     string
	resolution string
}

// writeFinalResult writes the terminal → done state for a match already locked in
// qtx: each player's score + Elo snapshot AND their users.rating (so the Elo reaches
// the ladder — not only match_players), then winner/reason/resolution on the match.
// Elo is applied here in exactly ONE place, shared by the judged and forfeit paths
// and keyed by user_id. It commits nothing — the caller owns the tx
// (docs/DESIGN-PHASE3-LIVE.md §2.4, §2.7).
func (s *Service) writeFinalResult(ctx context.Context, qtx *db.Queries, matchID string, fr finalResult) error {
	for _, p := range fr.players {
		before := int32(p.before)
		after := int32(p.after)
		if err := qtx.SetPlayerScore(ctx, db.SetPlayerScoreParams{
			MatchID: matchID, UserID: p.userID,
			Score: p.score, RatingBefore: &before, RatingAfter: &after,
		}); err != nil {
			return fmt.Errorf("game: set score: %w", err)
		}
		if err := qtx.UpdateUserRating(ctx, db.UpdateUserRatingParams{ID: p.userID, Rating: after}); err != nil {
			return fmt.Errorf("game: update rating: %w", err)
		}
	}

	reason := fr.reason
	resolution := fr.resolution
	if _, err := qtx.SetMatchResult(ctx, db.SetMatchResultParams{
		ID: matchID, WinnerPlayerID: fr.winner, JudgeReason: &reason, Resolution: &resolution,
	}); err != nil {
		return fmt.Errorf("game: set result: %w", err)
	}
	return nil
}

// persistResult writes both players' scores + Elo snapshots and the terminal
// done transition in one transaction — ratings are applied exactly once,
// atomically, on judging → done (docs/GAME.md §8).
func (s *Service) persistResult(ctx context.Context, matchID string, res judge.Result, winner *string, a, b playerResult) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("game: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	// Re-check status under the match lock so a second judging pass (a future
	// restart sweeper racing the live trigger) can't double-apply Elo: whoever
	// takes the lock first commits done; a loser sees status != judging and bails.
	m, err := qtx.GetMatchForUpdate(ctx, matchID)
	if err != nil {
		return fmt.Errorf("game: lock match: %w", err)
	}
	if m.Status != statusJudging {
		return nil
	}

	// Shared terminal writer (Elo in one place, seat-safe) — the judged path
	// (docs/DESIGN-PHASE3-LIVE.md §2.7). The forfeit path calls the same helper.
	if err := s.writeFinalResult(ctx, qtx, matchID, finalResult{
		winner:     winner,
		players:    []playerResult{a, b},
		reason:     res.Reason,
		resolution: resolutionJudged,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("game: commit tx: %w", err)
	}
	// Post-commit: both duelists' sockets get their per-viewer verdict. Reached only
	// when this pass actually wrote the result (the status != judging early return
	// above never gets here), so a losing double-judge does not double-publish
	// (docs/DESIGN-PHASE3-LIVE.md §3.2).
	s.publisher.Resolved(matchID)
	return nil
}

// ResultView is the end-of-round result (docs/API.md §8.4). Not-ready states
// carry only Status; a done match carries the full verdict.
type ResultView struct {
	Status       string
	Ready        bool
	PromptID     string
	PromptText   string
	WinnerUserID *string
	Reason       *string
	// Resolution is how the match was decided ('judged' | 'forfeit'); nil on legacy
	// rows (buildResultDTO defaults it to 'judged').
	Resolution *string
	Players    []ResultPlayer
}

// ResultPlayer is one player's revealed outcome (both are shown once done).
type ResultPlayer struct {
	UserID       string
	DisplayName  *string
	DrawingID    *string
	Score        *float64
	RatingBefore *int32
	RatingAfter  *int32
}

// Result returns the round result for a caller who must be a player (else
// ErrNotFound → hidden 404). Until the match is done it returns Ready=false with
// the current status; at done it reveals both drawings, scores, ratings, winner.
func (s *Service) Result(ctx context.Context, userID, matchID string) (ResultView, error) {
	m, err := s.q.GetMatch(ctx, matchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResultView{}, ErrNotFound
		}
		return ResultView{}, fmt.Errorf("game: get match: %w", err)
	}
	rows, err := s.q.ListMatchPlayers(ctx, matchID)
	if err != nil {
		return ResultView{}, fmt.Errorf("game: load roster: %w", err)
	}
	if !rowsContain(rows, userID) {
		return ResultView{}, ErrNotFound
	}
	if m.Status != statusDone {
		return ResultView{Status: m.Status, Ready: false}, nil
	}

	prompt, err := s.q.GetPromptByID(ctx, m.PromptID)
	if err != nil {
		return ResultView{}, fmt.Errorf("game: get prompt: %w", err)
	}
	players := make([]ResultPlayer, len(rows))
	for i, r := range rows {
		players[i] = ResultPlayer{
			UserID: r.UserID, DisplayName: r.DisplayName, DrawingID: r.DrawingID,
			Score: r.Score, RatingBefore: r.RatingBefore, RatingAfter: r.RatingAfter,
		}
	}
	return ResultView{
		Status: statusDone, Ready: true,
		PromptID: prompt.ID, PromptText: prompt.Text,
		WinnerUserID: m.WinnerPlayerID, Reason: m.JudgeReason,
		Resolution: m.Resolution,
		Players:    players,
	}, nil
}

// PlayerDrawing returns the vector document a fellow match participant submitted,
// for the result reveal. Authorization is match MEMBERSHIP (not drawing ownership,
// which 404s a non-owner and so can't serve the opponent's canvas), gated on the
// match being `done` (no peeking mid-duel). The single query folds all three trust
// gates; any miss — a non-member viewer, an unfinished match, a non-player target —
// yields pgx.ErrNoRows, which becomes ErrNotFound → a hidden 404 that leaks nothing
// (docs/API.md §8, docs/IDEAS.md). No object storage: the caller renders the
// returned document with the same editor renderer that draws the local canvas.
// (targetID == viewerID also works, so it's a uniform "participant drawing" read.)
func (s *Service) PlayerDrawing(ctx context.Context, viewerID, matchID, targetID string) (json.RawMessage, error) {
	doc, err := s.q.GetMatchPlayerDrawing(ctx, db.GetMatchPlayerDrawingParams{
		MatchID: matchID, TargetUserID: targetID, ViewerUserID: viewerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("game: get player drawing: %w", err)
	}
	return doc, nil
}

func rowsContain(rows []db.ListMatchPlayersRow, userID string) bool {
	for _, r := range rows {
		if r.UserID == userID {
			return true
		}
	}
	return false
}
