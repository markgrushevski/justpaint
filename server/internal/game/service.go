// Package game implements the async drawing-duel lifecycle: create/join, submit,
// judge out-of-band, and reveal the result (open → drawing → judging → done).
// Render and judge are seams (internal/render, internal/judge) with fake and real
// impls behind them. See docs/GAME.md, docs/API.md §8.
package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/markgrushevski/justpaint/server/internal/aibudget"
	"github.com/markgrushevski/justpaint/server/internal/db"
	"github.com/markgrushevski/justpaint/server/internal/document"
	"github.com/markgrushevski/justpaint/server/internal/judge"
	"github.com/markgrushevski/justpaint/server/internal/render"
)

// GameCanvasSize is the canonical square game canvas (docs/GAME.md §2), echoed
// to the client and enforced at submit (ValidateSubmission).
const GameCanvasSize = 1080

// modeAsync is the only match mode in v1; live realtime is a transport over the
// same lifecycle, not a second mode (docs/GAME.md §9).
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

// Match resolutions (docs/GAME.md §4.1): how a `done` match was decided;
// `abandoned` carries none. The DB check constraint (migration 00005) pins the
// same three.
const (
	resolutionJudged  = "judged"
	resolutionForfeit = "forfeit"
	// resolutionAborted: the judging pass never returned within maxJudgeAttempts, so
	// the round is closed unscored — no winner, no Elo (docs/DECISIONS.md 2026-09-18).
	resolutionAborted = "aborted"
)

// abortedReason is the player-facing judge_reason on an aborted round. The client
// branches on resolution == 'aborted', never on this text, but it is the only
// thing a player ever sees about why their duel produced no verdict.
const abortedReason = "this round could not be scored — the judge did not answer after several attempts, so no winner and no rating change were recorded"

// defaultJudgeConcurrency is the fallback judging-pass bound (see judgeLimiter):
// enough to keep a second duel moving while one renders, without thrashing a
// 512 MB instance running RENDER_MODE=node (two child processes per pass).
const defaultJudgeConcurrency = 2

// roundSeconds is the drawing-round length, stamped as drawing_deadline
// (now() + roundSeconds) when the roster fills. A Go constant, not a column, so
// changing it needs no migration (docs/GAME.md §4.1).
const roundSeconds = 90

// Sentinel errors the handler maps onto HTTP responses.
var (
	// ErrNoPrompts means no active prompt exists to pin — a seeding fault (→ 500);
	// run the seed migration (00002).
	ErrNoPrompts = errors.New("game: no active prompts to pin")
	// ErrNotFound means the match is absent or the caller isn't a player in it —
	// hidden as 404 so existence never leaks (docs/API.md §1, §8).
	ErrNotFound = errors.New("game: match not found")
	// ErrNotPlayer: authenticated but not a player in this match — maps to 403 on
	// submit (a known violation), unlike the read path's hidden 404 (API.md §8, submit).
	ErrNotPlayer = errors.New("game: not a player in this match")
	// ErrNotSubmittable: the match is not in the drawing state (already judging/
	// done/abandoned) → 409.
	ErrNotSubmittable = errors.New("game: match not accepting submissions")
	// ErrAlreadySubmitted: this player already submitted → 409 (no double-submit).
	ErrAlreadySubmitted = errors.New("game: already submitted")
	// ErrRoundExpired: the deadline passed before this submit landed. Not stamped —
	// the match resolves to forfeit/abandoned instead (409, docs/API.md §8, submit).
	ErrRoundExpired = errors.New("game: round deadline passed")
)

// PlayerRow is one roster slot, decoupled from the generated row type so the
// redaction logic (buildMatchDTO) stays pure and table-testable.
type PlayerRow struct {
	UserID      string
	DisplayName *string
	DrawingID   *string // nil until the player submits
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

// Service holds the game business logic: the pool (create/join + submit
// transactions), the generated queries, and the render/judge seams behind
// interfaces so a fake swaps for a real impl with no loop change. The logger is
// for the out-of-band judging goroutine, whose failures have no request to
// return to.
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	renderer render.Renderer
	judge    judge.Judge
	logger   *slog.Logger
	// publisher defaults to NopPublisher and is swapped for the ws hub via
	// SetPublisher, so NewService's signature and existing tests are unaffected.
	publisher Publisher
	// judging bounds concurrent judging passes across both dispatch paths (the
	// last submit and the sweeper). Never nil — both constructors build it.
	judging *judgeLimiter
	// checkBudget, billPlayers and billProvider are the duel's three calls into the
	// daily AI-call ceiling (internal/aibudget), bound to aibudget.KindDuel at the
	// composition root; nil means unbudgeted. Three, not a check/spend pair,
	// because the duel's two ledger facts are written at different moments — see
	// enterJudging (docs/GAME.md §4.3). Billing ports take tx-scoped queries; the
	// check does not.
	checkBudget  aibudget.Check
	billPlayers  aibudget.BillPlayers
	billProvider aibudget.BillProvider
}

// NewService builds the service with defaultJudgeConcurrency judging passes in
// flight. Prefer NewServiceWithConcurrency at the composition root; this
// shorthand is for tests and callers with no opinion.
func NewService(pool *pgxpool.Pool, q *db.Queries, renderer render.Renderer, jdg judge.Judge, logger *slog.Logger) *Service {
	return NewServiceWithConcurrency(pool, q, renderer, jdg, logger, defaultJudgeConcurrency)
}

// NewServiceWithConcurrency sets the judging-concurrency bound explicitly: at
// most judgeConcurrency passes (render + judge call) run at once, clamped to 1.
// Keeps RENDER_MODE=node from fork-bombing a small instance under a boot drain
// or a burst of final submits.
func NewServiceWithConcurrency(pool *pgxpool.Pool, q *db.Queries, renderer render.Renderer, jdg judge.Judge, logger *slog.Logger, judgeConcurrency int) *Service {
	return &Service{
		pool: pool, q: q, renderer: renderer, judge: jdg, logger: logger,
		publisher: NopPublisher{},
		judging:   newJudgeLimiter(judgeConcurrency),
	}
}

// CreateOrJoin is the single "play" entry point (docs/API.md §8 POST /api/matches,
// docs/DECISIONS.md 2026-07-03). In one transaction: (1) auto-joins the oldest
// waiting async match the caller isn't in, flipping it open→drawing; (2) failing
// that, returns the caller's own open match, so tapping "play" again doesn't
// stack duplicates; (3) failing that, creates a fresh open match with one random
// prompt pinned. Refuses with an aibudget error (→ 429) when the daily AI-call
// budget is out.
func (s *Service) CreateOrJoin(ctx context.Context, userID string) (MatchView, error) {
	// Checked ahead of the tx and ahead of all three branches: branch 3 bills the
	// joiner, not the creator, once someone joins later, so guarding only the join
	// would let a player over cap queue up and duel anyway. Outside the tx because
	// this is an advisory read and the tx below holds row locks worth keeping short.
	if s.checkBudget != nil {
		if err := s.checkBudget(ctx, userID); err != nil {
			return MatchView{}, err // an aibudget refusal; the handler maps it
		}
	}

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
		// Roster full → start the round and stamp the server-authoritative deadline
		// (now() + roundSeconds, DB clock) — replaces the generic flip here (GAME.md §4.1).
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
	// Bills both seats inside the same transaction that starts the round: "no
	// round, no grant" is atomic, and both players get it because both pressed
	// play. The provider bills separately, only at judging (enterJudging) —
	// starting a round is not yet a request to the judge (docs/GAME.md §4.3).
	if joined && s.billPlayers != nil {
		ids := make([]string, 0, len(view.Players))
		for _, p := range view.Players {
			ids = append(ids, p.UserID)
		}
		if err := s.billPlayers(ctx, qtx, ids...); err != nil {
			return MatchView{}, fmt.Errorf("game: bill duel players: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return MatchView{}, fmt.Errorf("game: commit tx: %w", err)
	}
	// Post-commit: the waiting player's socket learns the opponent joined and the
	// round started. Only the join branch flips state; reuse/create leave a
	// lone-open match nobody is watching yet (docs/API.md §9.2).
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

// Get returns match state for a caller who must be a player; a non-player or
// missing match is hidden as ErrNotFound (→ 404, docs/API.md §8). Read-only.
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

// assemble loads the pinned prompt and roster for m into a MatchView, on
// whatever queries it's given (tx or pool) so the snapshot matches its caller.
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
// §8.3): status now, and the caller's stored drawing id.
type SubmitResult struct {
	Status    string
	DrawingID string
	// Deadline is the round's absolute drawing deadline, echoed so the client can
	// re-anchor its countdown against the server clock after a submit.
	Deadline *time.Time
}

// Submit persists the caller's drawing, stamps their roster slot, and — if it's
// the last outstanding submission — flips the match to judging and scores it
// out-of-band (docs/GAME.md §4.1, docs/API.md §8, submit). The document is already
// validated + canvas-checked by the handler.
//
// Errors: ErrNotFound (404), ErrNotPlayer (403), ErrNotSubmittable (409),
// ErrAlreadySubmitted (409).
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
	// are not in is a known-ownership violation → ErrNotPlayer → 403 (API.md §8, submit).
	player, err := qtx.GetMatchPlayer(ctx, db.GetMatchPlayerParams{MatchID: matchID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmitResult{}, ErrNotPlayer
		}
		return SubmitResult{}, fmt.Errorf("game: get player: %w", err)
	}

	// Defense-in-depth on the DB clock (server_now, captured under this lock): a
	// submit landing after the deadline but before the next sweep tick would
	// otherwise still see status='drawing' and get stamped, silently turning a
	// forfeit into a judged match. Resolve the expiry here instead — not stamped
	// (docs/NOTES.md "Deadlines rely on now() being the transaction start time").
	if isExpiredDrawing(m) {
		outcome, err := s.resolveExpiry(ctx, qtx, m)
		if err != nil {
			return SubmitResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SubmitResult{}, fmt.Errorf("game: commit tx: %w", err)
		}
		if outcome == outcomeJudging {
			s.dispatchJudging(matchID)
		}
		// Uniform post-commit tail, identical to the sweeper's: notifies the winning
		// opponent, not on this request, the instant the late submit forfeits the
		// round (docs/API.md §9.2).
		s.publishOutcome(matchID, outcome)
		return SubmitResult{}, ErrRoundExpired
	}

	if m.Status != statusDrawing {
		return SubmitResult{}, ErrNotSubmittable
	}
	if player.SubmittedAt != nil {
		return SubmitResult{}, ErrAlreadySubmitted
	}

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
		if err := s.enterJudging(ctx, qtx, matchID); err != nil {
			return SubmitResult{}, err
		}
		status = statusJudging
		triggerJudging = true
	}

	if err := tx.Commit(ctx); err != nil {
		return SubmitResult{}, fmt.Errorf("game: commit tx: %w", err)
	}

	// Post-commit realtime: the room learns this player submitted (frame carries
	// {userId}; clients ignore their own) and, if this was the last submission,
	// that judging began (docs/API.md §9.2).
	s.publisher.PlayerSubmitted(matchID, userID)
	if triggerJudging {
		s.publisher.Judging(matchID)
		// Out-of-band: the response returns immediately (202 — docs/API.md §8, submit); a
		// crash mid-judge, or a dispatch the concurrency bound refuses, is recovered
		// by the stuck-judging sweep (sweeper.go).
		s.dispatchJudging(matchID)
	}
	return SubmitResult{Status: status, DrawingID: d.ID, Deadline: m.DrawingDeadline}, nil
}

// enterJudging flips a locked match into judging inside the caller's transaction
// and bills the provider for the request the flip causes. Every path into
// judging — the last submit, the deadline's both-submitted branch, the
// stuck-judging re-fire — goes through here, so "one provider row per pass" is
// guaranteed by the code (docs/GAME.md §4.3, docs/NOTES.md "Bill the ledger
// where the provider call happens"). It counts passes, not the judge's internal
// retries (docs/JUDGE.md §7 pins 3).
//
// SetMatchJudging also stamps judging_started_at + the attempt counter the
// stuck-judging watchdog uses (docs/GAME.md §4.1). A ledger failure fails the
// transition — staying in `drawing` beats a ceiling that silently stops counting.
func (s *Service) enterJudging(ctx context.Context, qtx *db.Queries, matchID string) error {
	if _, err := qtx.SetMatchJudging(ctx, matchID); err != nil {
		return fmt.Errorf("game: to judging: %w", err)
	}
	if s.billProvider == nil {
		return nil // unbudgeted (a fake judge spends no external quota)
	}
	if err := s.billProvider(ctx, qtx); err != nil {
		return fmt.Errorf("game: bill duel judge call: %w", err)
	}
	return nil
}

// dispatchJudging starts a judging pass if the concurrency bound allows one right
// now. It never blocks the caller and never queues: a refusal leaves the row in
// `judging` with its attempt stamped, for sweepStuckJudging to re-claim once it
// goes stale (docs/NOTES.md "Judging runs out of band and is recovered by the
// sweeper"). Returns whether the pass started, so a caller that already paid for
// a retry (the sweeper) can tell the difference.
func (s *Service) dispatchJudging(matchID string) bool {
	if s.judging.tryGo(func() { s.judgeMatch(matchID) }) {
		return true
	}
	s.logger.Warn("judging deferred: concurrency limit reached — the stuck-judging sweep will re-fire it",
		"matchID", matchID, "limit", s.judging.limit())
	return false
}

// JudgePassBudget bounds one judging pass end to end: two renders plus the judge
// call, retries included. It must clear the judge's whole retry envelope
// (docs/JUDGE.md §7 pins 3 attempts, so JUDGE_TIMEOUT=10s alone needs 30s) or the
// wrapper silently truncates the last retry (docs/NOTES.md "JudgePassBudget must
// fit the judge's retry envelope"). Generous on purpose: a wedged pass is caught
// by the stuck-judging sweep and resolved as done/'aborted' (docs/GAME.md §4.1).
const JudgePassBudget = 60 * time.Second

// judgeMatch runs the judging pass for a match that just entered judging, on its
// own background context. Runs holding a judgeLimiter slot — start it via
// dispatchJudging or judging.goHeld, never a bare `go`.
func (s *Service) judgeMatch(matchID string) {
	ctx, cancel := context.WithTimeout(context.Background(), JudgePassBudget)
	defer cancel()
	if err := s.runJudging(ctx, matchID); err != nil {
		// Quota exhaustion is an operational fact, not a bug: every duel will fail
		// the same way until the budget resets. Name it so the cause is a grep
		// away instead of an evening spent suspecting the judge.
		if errors.Is(err, judge.ErrQuotaExhausted) {
			s.logger.Error("judge quota exhausted — every duel will abort until the budget resets",
				"matchID", matchID, "err", err)
			return
		}
		// Leaves the match in judging: the stuck-judging sweep re-fires it until
		// maxJudgeAttempts, then closes it as done/'aborted' rather than wedging
		// forever (sweeper.go, docs/GAME.md §4.1).
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

	startedAt := time.Now()
	res, err := s.judge.Score(ctx, judge.Request{Prompt: prompt.Text, ImageA: imgA, ImageB: imgB})
	if err != nil {
		return fmt.Errorf("game: judge: %w", err)
	}
	if err := res.Validate(); err != nil {
		return fmt.Errorf("game: judge result: %w", err)
	}
	// The one line showing a live judge produces sane verdicts, not every duel a
	// tie or every score 0, and that latency isn't creeping toward the pass budget.
	// Scores and latency only — the reason is player-facing text, not a log field.
	s.logger.Info("match judged",
		"matchID", matchID,
		"prompt", prompt.Text,
		"scoreA", res.ScoreA,
		"scoreB", res.ScoreB,
		"winner", res.Winner,
		"judge_ms", time.Since(startedAt).Milliseconds(),
		"render_bytes", len(imgA)+len(imgB),
	)

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
// judge score (nil on a forfeit), the Elo snapshot, keyed by user_id.
type playerResult struct {
	userID string
	score  *float64
	before int
	after  int
}

// finalResult is the seat-independent terminal state the judged (persistResult)
// and forfeit (resolveExpiry) paths hand to writeFinalResult. winner is nil on a
// tie; players are keyed by user_id, never seat index, so Elo can't land on the
// wrong seat.
type finalResult struct {
	winner     *string
	players    []playerResult
	reason     string
	resolution string
}

// writeFinalResult writes the terminal → done state for a match already locked
// in qtx: each player's score + Elo snapshot and users.rating, then
// winner/reason/resolution. Elo is applied in exactly one place, shared by the
// judged and forfeit paths, keyed by user_id. Commits nothing — the caller owns
// the tx (docs/GAME.md §8).
//
// The ladder move is atomic (`rating = rating + delta` returning the new value,
// never SET), because the match-row lock serializes per match, not per user.
// before/after are derived from that RETURNING, not the pre-match read, so the
// snapshot holds under concurrency (docs/NOTES.md "Ratings move by an atomic
// delta").
func (s *Service) writeFinalResult(ctx context.Context, qtx *db.Queries, matchID string, fr finalResult) error {
	// Lock the user rows in a global (user_id) order: ApplyRatingDelta holds each
	// row lock until commit, so a rematch resolving concurrently would deadlock
	// (40P01) if the two matches took the locks in opposite orders.
	slices.SortFunc(fr.players, func(a, b playerResult) int {
		return strings.Compare(a.userID, b.userID)
	})

	for _, p := range fr.players {
		// Rating write first so its RETURNING feeds the snapshot; before/after stay
		// per-iteration locals since SetPlayerScore takes their addresses.
		delta := int32(p.after - p.before)
		after, err := qtx.ApplyRatingDelta(ctx, db.ApplyRatingDeltaParams{ID: p.userID, Delta: delta})
		if err != nil {
			return fmt.Errorf("game: apply rating delta: %w", err)
		}
		before := after - delta
		if err := qtx.SetPlayerScore(ctx, db.SetPlayerScoreParams{
			MatchID: matchID, UserID: p.userID,
			Score: p.score, RatingBefore: &before, RatingAfter: &after,
		}); err != nil {
			return fmt.Errorf("game: set score: %w", err)
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

	// Re-check status under the match lock so a second judging pass (the
	// stuck-judging sweeper racing the live trigger) can't double-apply Elo: whoever
	// takes the lock first commits done; a loser sees status != judging and bails.
	m, err := qtx.GetMatchForUpdate(ctx, matchID)
	if err != nil {
		return fmt.Errorf("game: lock match: %w", err)
	}
	if m.Status != statusJudging {
		return nil
	}

	// Shared terminal writer (Elo in one place, seat-safe) — the judged path
	// (docs/GAME.md §8). The forfeit path calls the same helper.
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
	// Post-commit: both duelists get their per-viewer verdict. Only reached when
	// this pass actually wrote the result, so a losing double-judge never
	// double-publishes (docs/API.md §9.2).
	s.publisher.Resolved(matchID)
	return nil
}

// ResultView is the end-of-round result (docs/API.md §8, result). Not-ready states
// carry only Status; a done match carries the full verdict.
type ResultView struct {
	Status       string
	Ready        bool
	PromptID     string
	PromptText   string
	WinnerUserID *string
	Reason       *string
	// Resolution is how the match was decided ('judged' | 'forfeit' | 'aborted'); nil
	// on legacy rows (buildResultDTO defaults it to 'judged').
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

// PlayerDrawing returns a fellow participant's submitted document (targetID ==
// viewerID also works, as a uniform read). Authorization is match membership,
// not drawing ownership (which 404s a non-owner), gated on the match being
// `done` — no peeking mid-duel. One query folds all three trust gates; any miss
// becomes ErrNotFound, a hidden 404 (docs/API.md §8). No object storage: the
// caller renders the document with the same renderer as the local canvas.
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

// SetBudget installs the duel's three ports into the daily AI-call ceiling, the
// same post-construction wiring as SetPublisher, so NewService's signature and
// existing tests are unaffected. Any port may be nil (unbudgeted); deciding
// whether a real provider exists is the composition root's job.
func (s *Service) SetBudget(check aibudget.Check, billPlayers aibudget.BillPlayers, billProvider aibudget.BillProvider) {
	s.checkBudget = check
	s.billPlayers = billPlayers
	s.billProvider = billProvider
}
