// Package game implements the async drawing-duel lifecycle (open → drawing → judging
// → done): create/join, submit, out-of-band judging and the result. See docs/GAME.md
// and docs/API.md §8.
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

// GameCanvasSize is the square game canvas (docs/GAME.md §2), enforced at submit.
const GameCanvasSize = 1080

// modeAsync is the only mode: live realtime is a transport, not a mode (docs/GAME.md §9).
const modeAsync = "async"

// Match statuses (docs/GAME.md §3); the DB check constraint holds the full enum.
const (
	statusOpen      = "open"
	statusDrawing   = "drawing"
	statusJudging   = "judging"
	statusDone      = "done"
	statusAbandoned = "abandoned"
)

// Match resolutions: how a `done` match was decided (docs/GAME.md §4.1). An aborted
// round got no verdict within maxJudgeAttempts, so it has no winner and no Elo.
const (
	resolutionJudged  = "judged"
	resolutionForfeit = "forfeit"
	resolutionAborted = "aborted"
)

// abortedReason is player-facing only; the client branches on the resolution.
const abortedReason = "this round could not be scored — the judge did not answer after several attempts, so no winner and no rating change were recorded"

// defaultJudgeConcurrency keeps a second duel moving without thrashing a 512 MB
// instance on RENDER_MODE=node, which spawns two child processes per pass.
const defaultJudgeConcurrency = 2

// roundSeconds is the drawing round, stamped as a DB-clock deadline when the roster fills.
const roundSeconds = 90

// Sentinel errors the handler maps onto HTTP responses.
var (
	// ErrNoPrompts is a seeding fault (500): no active prompt exists.
	ErrNoPrompts = errors.New("game: no active prompts to pin")
	// ErrNotFound covers a missing match and a non-player alike (404), so existence
	// never leaks.
	ErrNotFound = errors.New("game: match not found")
	// ErrNotPlayer is submit's 403, unlike the read path's hidden 404 (docs/API.md §8, submit).
	ErrNotPlayer        = errors.New("game: not a player in this match")
	ErrNotSubmittable   = errors.New("game: match not accepting submissions") // 409
	ErrAlreadySubmitted = errors.New("game: already submitted")               // 409
	// ErrRoundExpired (409): the deadline passed first, so the submit is not stamped and
	// the match resolves as a forfeit or abandoned instead.
	ErrRoundExpired = errors.New("game: round deadline passed")
)

// PlayerRow is one roster slot, separate from the generated row so buildMatchDTO stays
// table-testable.
type PlayerRow struct {
	UserID      string
	DisplayName *string
	DrawingID   *string // nil until the player submits
}

// MatchView is the assembled match state; the handler applies per-viewer visibility.
type MatchView struct {
	ID              string
	Mode            string
	Status          string
	PromptID        string
	PromptText      string
	Players         []PlayerRow
	DrawingDeadline *time.Time // DB clock; nil while `open`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Service holds the duel logic. The logger serves the out-of-band judging goroutine,
// whose failures have no request to return to.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	renderer  render.Renderer
	judge     judge.Judge
	logger    *slog.Logger
	publisher Publisher     // NopPublisher until SetPublisher installs the ws hub
	judging   *judgeLimiter // shared by the last submit and the sweeper; never nil
	// The duel's AI-budget ports; nil means unbudgeted. Its player and provider halves
	// are billed at different moments (docs/GAME.md §4.3).
	checkBudget  aibudget.Check
	billPlayers  aibudget.BillPlayers
	billProvider aibudget.BillProvider
}

// NewService builds the service with defaultJudgeConcurrency.
func NewService(pool *pgxpool.Pool, q *db.Queries, renderer render.Renderer, jdg judge.Judge, logger *slog.Logger) *Service {
	return NewServiceWithConcurrency(pool, q, renderer, jdg, logger, defaultJudgeConcurrency)
}

// NewServiceWithConcurrency runs at most judgeConcurrency passes (renders + judge
// call) at once, clamped to 1, so RENDER_MODE=node cannot fork-bomb a small instance.
func NewServiceWithConcurrency(pool *pgxpool.Pool, q *db.Queries, renderer render.Renderer, jdg judge.Judge, logger *slog.Logger, judgeConcurrency int) *Service {
	return &Service{
		pool: pool, q: q, renderer: renderer, judge: jdg, logger: logger,
		publisher: NopPublisher{},
		judging:   newJudgeLimiter(judgeConcurrency),
	}
}

// CreateOrJoin is the one "play" entry point (docs/API.md §8). In one transaction it
// (1) joins the oldest open match the caller is not in, else (2) returns the caller's
// own open match, else (3) creates one. An exhausted AI budget refuses first.
func (s *Service) CreateOrJoin(ctx context.Context, userID string) (MatchView, error) {
	// Before every branch, not just the join: a creator is billed only when someone
	// joins, so a player over cap could otherwise queue a match and duel anyway.
	// Outside the tx, since it is advisory and the tx holds row locks.
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
		// Roster full: start the round with a DB-clock deadline.
		if m, err = qtx.SetMatchDrawing(ctx, db.SetMatchDrawingParams{ID: m.ID, RoundSeconds: roundSeconds}); err != nil {
			return MatchView{}, fmt.Errorf("game: start match: %w", err)
		}
		joined = true
	case errors.Is(err, pgx.ErrNoRows):
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
	// Both seats are billed in the transaction that starts the round, so no round means
	// no charge. The provider is billed later, at judging (enterJudging).
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
	// Only a join changes state that someone is watching.
	if joined {
		s.publisher.MatchChanged(m.ID)
	}
	return view, nil
}

// createMatch pins one random active prompt and seats the creator.
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

// Get returns match state to a player; anyone else gets ErrNotFound.
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

// assemble loads m's prompt and roster through q, a tx or the pool.
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

// SubmitResult is what the submit handler echoes (docs/API.md §8, submit).
type SubmitResult struct {
	Status    string
	DrawingID string
	Deadline  *time.Time // lets the client re-anchor its countdown on the server clock
}

// Submit stores the caller's drawing and stamps their slot; the last submission flips
// the match to judging and scores it out of band (docs/GAME.md §4.1). The handler has
// already validated doc.
func (s *Service) Submit(ctx context.Context, userID, matchID string, doc document.Document, raw []byte) (SubmitResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("game: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	// The lock keeps two simultaneous final submits from both missing "I'm last".
	m, err := qtx.GetMatchForUpdate(ctx, matchID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmitResult{}, ErrNotFound
		}
		return SubmitResult{}, fmt.Errorf("game: lock match: %w", err)
	}

	player, err := qtx.GetMatchPlayer(ctx, db.GetMatchPlayerParams{MatchID: matchID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SubmitResult{}, ErrNotPlayer
		}
		return SubmitResult{}, fmt.Errorf("game: get player: %w", err)
	}

	// Past the deadline but before the next sweep, the match still reads 'drawing';
	// stamping it would turn a forfeit into a judged match. Resolve the expiry instead
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

	// The `submitted_at is null` guard makes a racing double-tap stamp nothing.
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

	s.publisher.PlayerSubmitted(matchID, userID)
	if triggerJudging {
		s.publisher.Judging(matchID)
		// A crash mid-judge or a refused dispatch is recovered by the sweeper.
		s.dispatchJudging(matchID)
	}
	return SubmitResult{Status: status, DrawingID: d.ID, Deadline: m.DrawingDeadline}, nil
}

// enterJudging flips a locked match to judging and bills the provider, in the
// caller's tx. Every path into judging goes through here, so each pass bills exactly
// one provider row (docs/GAME.md §4.3). A ledger failure fails the flip: staying in
// `drawing` beats a ceiling that silently stops counting.
func (s *Service) enterJudging(ctx context.Context, qtx *db.Queries, matchID string) error {
	if _, err := qtx.SetMatchJudging(ctx, matchID); err != nil {
		return fmt.Errorf("game: to judging: %w", err)
	}
	if s.billProvider == nil {
		return nil // unbudgeted
	}
	if err := s.billProvider(ctx, qtx); err != nil {
		return fmt.Errorf("game: bill duel judge call: %w", err)
	}
	return nil
}

// dispatchJudging starts a pass if a concurrency slot is free and reports whether it
// did. It never blocks or queues: a refused pass stays in `judging` for the sweeper
// to re-fire once stale.
func (s *Service) dispatchJudging(matchID string) bool {
	if s.judging.tryGo(func() { s.judgeMatch(matchID) }) {
		return true
	}
	s.logger.Warn("judging deferred: concurrency limit reached — the stuck-judging sweep will re-fire it",
		"matchID", matchID, "limit", s.judging.limit())
	return false
}

// JudgePassBudget bounds one judging pass: two renders plus the judge call with all
// its retries (docs/NOTES.md "JudgePassBudget must fit the judge's retry envelope").
const JudgePassBudget = 60 * time.Second

// judgeMatch runs one pass on a background context, holding a judgeLimiter slot:
// start it through dispatchJudging or judging.goHeld, never a bare `go`.
func (s *Service) judgeMatch(matchID string) {
	ctx, cancel := context.WithTimeout(context.Background(), JudgePassBudget)
	defer cancel()
	if err := s.runJudging(ctx, matchID); err != nil {
		if errors.Is(err, judge.ErrQuotaExhausted) {
			s.logger.Error("judge quota exhausted — every duel will abort until the budget resets",
				"matchID", matchID, "err", err)
			return
		}
		// The match stays in judging for the sweeper to re-fire or abort.
		s.logger.Error("judge match", "matchID", matchID, "err", err)
	}
}

// runJudging renders both submissions, scores them, applies Elo and finishes the
// match. Its status check is not a lock; persistResult re-checks under one.
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

	// subs[0] is image A and subs[1] image B, by the query's order (GAME.md §7.1).
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
	// Scores and latency only: the reason is player-facing text, not a log field.
	s.logger.Info("match judged",
		"matchID", matchID,
		"prompt", prompt.Text,
		"scoreA", res.ScoreA,
		"scoreB", res.ScoreB,
		"winner", res.Winner,
		"judge_ms", time.Since(startedAt).Milliseconds(),
		"render_bytes", len(imgA)+len(imgB),
	)

	// Positional winner to player id (nil on a tie), and A's Elo score.
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
		// the defaults above
	}

	ratingA, ratingB := int(subs[0].Rating), int(subs[1].Rating)
	afterA, afterB := computeElo(ratingA, ratingB, sa)

	return s.persistResult(ctx, matchID, res, winner,
		playerResult{userID: subs[0].UserID, score: &res.ScoreA, before: ratingA, after: afterA},
		playerResult{userID: subs[1].UserID, score: &res.ScoreB, before: ratingB, after: afterB},
	)
}

// renderSubmission re-parses a stored document (validated at submit, so a failure is
// corruption) and renders the judged raster.
func (s *Service) renderSubmission(ctx context.Context, raw []byte) ([]byte, error) {
	doc, err := document.ParseAndValidate(raw)
	if err != nil {
		return nil, fmt.Errorf("game: reparse submission: %w", err)
	}
	return s.renderer.Render(ctx, doc)
}

// playerResult is one player's terminal record; score is nil on a forfeit.
type playerResult struct {
	userID string
	score  *float64
	before int
	after  int
}

// finalResult is the terminal state the judged and forfeit paths share. Players are
// keyed by user_id, never seat, so Elo cannot land on the wrong seat.
type finalResult struct {
	winner     *string
	players    []playerResult
	reason     string
	resolution string
}

// writeFinalResult writes the `done` state of a match locked in qtx; it is the one
// place Elo is applied, and it commits nothing. Ratings move by an atomic delta since
// the match lock is per match, not per user
// (docs/NOTES.md "Ratings move by an atomic delta").
func (s *Service) writeFinalResult(ctx context.Context, qtx *db.Queries, matchID string, fr finalResult) error {
	// Lock user rows in user_id order: each lock is held until commit, so a concurrent
	// rematch taking them in the opposite order would deadlock.
	slices.SortFunc(fr.players, func(a, b playerResult) int {
		return strings.Compare(a.userID, b.userID)
	})

	for _, p := range fr.players {
		// Rating write first, so its RETURNING feeds the snapshot.
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

// persistResult writes the judged result in one transaction.
func (s *Service) persistResult(ctx context.Context, matchID string, res judge.Result, winner *string, a, b playerResult) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("game: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	// Of two racing passes (live and re-fired), the first to lock commits and the other
	// bails here, so Elo applies once.
	m, err := qtx.GetMatchForUpdate(ctx, matchID)
	if err != nil {
		return fmt.Errorf("game: lock match: %w", err)
	}
	if m.Status != statusJudging {
		return nil
	}

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
	s.publisher.Resolved(matchID)
	return nil
}

// ResultView is the round result (docs/API.md §8, result); before `done` only Status
// is set.
type ResultView struct {
	Status       string
	Ready        bool
	PromptID     string
	PromptText   string
	WinnerUserID *string
	Reason       *string
	Resolution   *string // nil on legacy rows; buildResultDTO reads that as 'judged'
	Players      []ResultPlayer
}

// ResultPlayer is one player's revealed outcome.
type ResultPlayer struct {
	UserID       string
	DisplayName  *string
	DrawingID    *string
	Score        *float64
	RatingBefore *int32
	RatingAfter  *int32
}

// Result returns the round result to a player (else ErrNotFound); before `done` it
// returns Ready=false with the status.
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

// PlayerDrawing returns a participant's submitted document to another participant
// once the match is `done`. It authorizes by match membership, not drawing ownership;
// one query holds every gate, and any miss is ErrNotFound.
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

// SetBudget installs the duel's AI-budget ports after construction, like
// SetPublisher. A nil port is unbudgeted.
func (s *Service) SetBudget(check aibudget.Check, billPlayers aibudget.BillPlayers, billProvider aibudget.BillProvider) {
	s.checkBudget = check
	s.billPlayers = billPlayers
	s.billProvider = billProvider
}
