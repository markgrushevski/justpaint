<!-- DESIGN PROPOSAL — drafted 2026-07-11. Server-authoritative round deadline (forfeit-judge) + WS realtime, the back half of Phase 3. Defaults in §5 ACCEPTED by owner 2026-07-11 (see banner there). On approval, fold decisions into GAME.md / API.md / ARCHITECTURE.md / DECISIONS.md and delete this draft. -->

# justpaint Phase 3 — Server Deadlines & Live Realtime: Combined Design

## 1. Executive summary

Phase 3's async duel runs end-to-end today, but two gaps remain before it feels like a game: the round timer is a **client-only** `ROUND_SECONDS = 90` fiction that drifts per-player and is never reconciled with the server, and a match whose deadline passes has **no code path at all** — `status='abandoned'` is a valid enum with zero writers, and forfeit is unimplemented. This document designs the two subsystems that close both, as one coherent build:

- **Round-Deadline subsystem** (`internal/game`) — the server stamps an absolute `drawing_deadline` from Postgres' clock at `open→drawing`, exposes it (plus a `serverTime` anchor) on the match DTO, enforces it at `Submit` **against the DB clock**, and sweeps it with a background ticker so resolution never depends on a client polling. One lock-guarded routine, `resolveExpiry`, fans out to **forfeit-judge** (1 submitter → default win, ML judge does not run), **normal judging** (both submitted), or **abandoned** (nobody drew), and every caller publishes the committed outcome through one uniform post-commit tail. No new dependency.
- **WS-Realtime subsystem** (`internal/ws`, `coder/websocket`) — a single-process in-memory hub that pushes match-room state (presence, "opponent submitted", the deadline, judging, verdict) to both duelists, while Postgres stays authoritative and the REST poll loop is demoted to a throttled fallback. The hub owns zero authoritative state and never mediates a mutation; it only fans out read-only snapshots of transitions the deadline subsystem already committed. **Per-viewer visibility is preserved by building `match_state` per recipient** — the same runtime redaction REST uses — not by a marshal-once broadcast.

**They are ordered, not independent:** the deadline subsystem produces the server-authoritative facts (the deadline instant, the forfeit/abandon outcome); the WS layer is the low-latency delivery of exactly those facts. Build the deadline subsystem first — it ships standalone value over the existing poll loop with no new `go.mod` dependency, and the WS layer is literally uninteresting (and cannot be correct) without it.

Where the two source designs diverged, this document makes the call and says so inline (persist a typed `resolution` column; keep the explicit `Submit` expiry check but do it **DB-side**; leave forfeit similarity `score` null; sweep at 3s).

---

## 2. Round-Deadline subsystem (`internal/game`)

### 2.1 Headline decisions

| Question | Decision | Rationale |
|---|---|---|
| Trigger | Submit-time check **+ background sweeper** (3s tick + boot sweep). Reads stay pure. | Lazy-only leaves matches stuck forever if nobody polls — a real correctness hole. The sweeper is cheap behind a partial index and also fixes crash-mid-judge. |
| Resolve on GET reads? | **No** — reads are side-effect-free. | Poll at 2s + sweep at 3s ⇒ resolution visible within ≤3s regardless; WS push makes it instant. |
| Deadline value | Absolute `timestamptz`, set via Postgres `now() + interval`. | One clock authority — no Go-host/DB skew in the comparison every reader makes. |
| Clock used to **enforce** expiry | **Postgres `now()`**, both in the sweeper's SQL and in `Submit` (via the row-lock read that also returns `server_now`). | The "one clock authority" premise must hold for the one path that *rejects a user*, not just for readers. Comparing a Postgres-stamped deadline against the Go host wall clock reintroduces exactly the skew this design eliminates. |
| Round length | Go constant `roundDuration = 90 * time.Second`, **not** a column. | Invariant today; changing it needs no migration. A per-prompt length later is an additive `00005`. |
| Judge on forfeit? | **No** — fixed `sa = 1.0` into existing `computeElo`. | The `Judge` seam takes two rendered images; a forfeit has one document, so calling it is undefined, not a degenerate case. A default win must not be quality-weighted. |
| Late submit | **Reject (409 `round_expired`), no grace window**, don't stamp it. | Strictest defensible reading of the locked "both submitted *at* the deadline" = state-at-resolution. |
| Distinguish forfeit in UI | Persisted `resolution` column, surfaced as a typed DTO field. | Robust vs. regex-sniffing the human-prose `judge_reason`. |

### 2.2 Schema & migration

`server/migrations/00004_round_deadline.sql`:

```sql
-- +goose Up
alter table matches
    add column drawing_deadline   timestamptz,
    add column resolution         text check (resolution in ('judged', 'forfeit')),
    add column judge_attempts      int  not null default 0,
    add column judging_started_at  timestamptz;

-- Backfill BEFORE indexing so no existing row is stranded or mistyped:
--   * live 'drawing' rows with a null deadline would never be swept (immortal) — give them one now.
--   * historical 'done' rows must not present resolution=NULL to a non-null DTO field.
update matches set drawing_deadline = now() + make_interval(secs => 90)
    where status = 'drawing' and drawing_deadline is null;
update matches set resolution = 'judged'
    where status = 'done' and resolution is null;

-- Sweeper scans only live, already-expired rows.
create index matches_deadline_sweep_idx
    on matches (drawing_deadline) where status = 'drawing';

-- Watchdog scans only stuck judging rows.
create index matches_judging_stuck_idx
    on matches (judging_started_at) where status = 'judging';

-- Open-match reaper scans only never-joined rows.
create index matches_open_stale_idx
    on matches (created_at) where status = 'open';

-- +goose Down
drop index if exists matches_open_stale_idx;
drop index if exists matches_judging_stuck_idx;
drop index if exists matches_deadline_sweep_idx;
alter table matches
    drop column judging_started_at,
    drop column judge_attempts,
    drop column resolution,
    drop column drawing_deadline;
```

- `drawing_deadline` — null while `open`; stamped once at `open → drawing` as `now() + roundDuration`. Backfilled for any match caught mid-`drawing` by the migration so it can still be resolved.
- `resolution` — set only on terminal `done` rows: `'judged'` or `'forfeit'`. Null for `abandoned` (no result). Backfilled to `'judged'` for pre-migration `done` rows so the DTO field is never null on a completed match.
- `judge_attempts` — retry counter for the stuck-judging watchdog.
- `judging_started_at` — stamped each time a judge attempt *begins* (at `→judging` and on every watchdog re-fire). The watchdog measures staleness against **this**, not `updated_at`, so a genuinely in-flight judge does not look stuck (see §2.6). Bundled because the sweeper is the vehicle and the codebase already flags the crash-mid-judge gap (`service.go:338`). Clearly adjacent scope — see §2.6.
- **No `match_players` change:** `submitted_at`, `score`, `rating_before/after`, `winner_player_id`, `judge_reason` already carry a forfeit outcome exactly like a judged one.

New sqlc queries in `matches.sql` (then `sqlc generate`):

- `GetMatchForUpdate` — extend the existing `select *` to `select *, now() as server_now` so every locked read hands the caller the **DB clock** for expiry comparison under the same lock. This is the single change that keeps enforcement on the DB's clock.
- `SetMatchDrawing(id, round_seconds)` → `set status='drawing', drawing_deadline = now() + make_interval(secs => $2), updated_at = now() returning *`. Replaces the generic `UpdateMatchStatus(...,"drawing")` **only** at the `open→drawing` site.
- `SetMatchJudging(id)` → `set status='judging', judging_started_at = now(), judge_attempts = judge_attempts + 1, updated_at = now()`. Used both at last-submit and by the watchdog re-fire, so `judging_started_at` always reflects the current attempt.
- `GetMatchPlayersForResolve(match_id)` → per-player `{user_id, submitted_at, drawing_id, rating}` (joins `users.rating`). One read gives submitted-count, seat identity, and ratings for Elo.
- `ListExpiredDrawingMatches(limit)` → `where status='drawing' and drawing_deadline <= now() order by drawing_deadline limit $1 for update skip locked`.
- `ListStuckJudgingMatches(stale_interval, max_attempts, limit)` → `where status='judging' and judging_started_at <= now() - $1 and judge_attempts < $2 ... for update skip locked`.
- `ListStaleOpenMatches(ttl_interval, limit)` → `where status='open' and created_at <= now() - $1 ... for update skip locked` (open-match reaper, §2.6).
- Extend `SetMatchResult` to also set `resolution`; add `SetMatchAbandoned(id)` (status → `abandoned`, no result columns touched).

### 2.3 Lifecycle

```
open ──(roster fills)──▶ drawing [deadline = now()+90s]
  │                         │
 (never joined,   ┌─────────┼─────────────────────────┐
  TTL sweep)  both submit   deadline passes       deadline passes
  │            before dl     (1 submitter)          (0 submitters)
  ▼               │              │                       │
abandoned         ▼              ▼                       ▼
(no result)    judging ─▶ done  FORFEIT: done         abandoned
               (ML judge,        (no judge, winner=    (no result,
                resolution=       submitter, Elo S=1/0, no Elo)
                'judged')         resolution='forfeit')
```

`judging` is **optional** — the forfeit path goes `drawing → done` directly and never enters it. Every consumer (including the hub's frame machine) must treat `judging` as a skippable state.

The deadline is written at exactly one place: the `FindOpenMatchToJoin` hit in `CreateOrJoin` (`service.go:124`), swapping `UpdateMatchStatus(...,"drawing")` for `SetMatchDrawing(id, 90)`. The solo-create branch (`createMatch`) stays `open` with a null deadline — and is now bounded by the open-match reaper (§2.6). The `drawing → judging` flip in `Submit` becomes `SetMatchJudging` (stamps the attempt) but still doesn't read the deadline.

### 2.4 Enforcement — one shared routine

`internal/game/deadline.go`:

```go
type resolveOutcome int
const (
    outcomeNone resolveOutcome = iota // not expired / not our row → no transition
    outcomeAbandoned                  // 0 submitted
    outcomeForfeit                    // 1 submitted → done
    outcomeJudging                    // 2 submitted (defensive) → judging
)

// resolveExpiry runs with the match row already FOR UPDATE-locked in qtx and the
// DB clock (serverNow) captured under that same lock. Recheck-then-act; safe to
// call redundantly. It commits nothing and publishes nothing — the caller does both.
func (s *Service) resolveExpiry(ctx, qtx, m db.Match, serverNow time.Time) (resolveOutcome, error)
```

Internals (inside caller's lock):
1. Return `outcomeNone` if `m.Status != statusDrawing` or `serverNow.Before(*m.DrawingDeadline)` (idempotency guard using the **DB clock**, mirroring `persistResult`'s `status != judging` recheck at `:444`).
2. `rows := GetMatchPlayersForResolve(m.ID)`; `n := count(submitted_at != nil)`.
3. Branch:
   - **n == 0** → `SetMatchAbandoned(m.ID)`; return `outcomeAbandoned`.
   - **n == 1** → forfeit via the shared `writeFinalResult` helper (see §2.7); return `outcomeForfeit`.
   - **n == 2** → defensive only (`Submit` flips to `judging` in the same tx that stamps the 2nd submit, and the sweep's `WHERE status='drawing'` excludes it — unreachable in steady state). `SetMatchJudging(id)`, `slog.Warn`, return `outcomeJudging`.

**Uniform post-commit tail.** Every caller of `resolveExpiry` runs the *same* two lines after a successful commit, so a resolution is always both judged-fired (if needed) and published, no matter which caller triggered it:

```go
if outcome == outcomeJudging { go s.judgeMatch(matchID) }
s.publishOutcome(matchID, outcome)   // maps outcome → result / abandoned / judging frame; NopPublisher by default
```

`publishOutcome` rebuilds the now-terminal state from the same gated read the REST result handler uses and fans it out (§3). It is a no-op for `outcomeNone`.

**Caller 1 — `Submit` (`service.go:245`, defense-in-depth).** After the locked `GetMatchForUpdate` (which now also returns `serverNow`) confirms the player, *before* the `player.SubmittedAt` check:

```go
if m.Status == statusDrawing && m.DrawingDeadline != nil && !serverNow.Before(*m.DrawingDeadline) {
    outcome, err := s.resolveExpiry(ctx, qtx, m, serverNow)   // late submit NOT stamped
    if err != nil { return SubmitResult{}, err }
    if err := tx.Commit(ctx); err != nil { return SubmitResult{}, err }
    if outcome == outcomeJudging { go s.judgeMatch(matchID) }
    s.publishOutcome(matchID, outcome)                        // <-- winner IS notified from this path too
    return SubmitResult{}, ErrRoundExpired                    // → 409 {status, reason:"round_expired"}
}
```

> **Reconciliation (the one real disagreement between the source designs):** the WS design argued `Submit` needs no change because its existing `status != statusDrawing` recheck already covers the race. That is insufficient. A submit landing *after* the deadline but *before* the next 3s sweep tick still sees `status='drawing'` and would be stamped — silently converting an opponent's forfeit-loss into a judged match. The explicit `serverNow >= deadline` check is required and wins. The comparison uses the **DB clock captured under the lock**, not `time.Now()`, so a fast/slow Go host cannot 409 a submission the database still considers live (or vice-versa). The late submission is **never attached**; a player 2s late cannot steal a forfeit result. And because the publish is in this path's tail, the *winning* opponent (not on this request) is notified immediately, not after the demoted poll cadence.

**Caller 2 — background sweeper, `internal/game/sweeper.go`:**

```go
func (s *Service) RunSweeper(ctx, interval) {
    s.sweepOnce(ctx, drainToEmpty)   // boot pass: loop each phase until its backlog is empty
    t := time.NewTicker(interval); defer t.Stop()
    for { select {
        case <-ctx.Done(): return
        case <-t.C: s.sweepOnce(ctx, oneBatch)   // steady state: one page per phase per tick
    }}
}
func (s *Service) sweepOnce(ctx, mode) {
    s.sweepExpiredDrawing(ctx, mode)
    s.sweepStuckJudging(ctx, mode)
    s.sweepStaleOpen(ctx, mode)
}
```

`sweepExpiredDrawing` selects ids via `ListExpiredDrawingMatches` (`FOR UPDATE SKIP LOCKED`, page size ~256), then resolves **each in its own tx** (`Begin → GetMatchForUpdate → resolveExpiry → Commit`, then the uniform post-commit tail — judge-fire + publish). One bad row logs and retries next tick; it can't stall the batch. Wired in `cmd/server/main.go`: `go svc.RunSweeper(ctx, 3*time.Second)` on the shutdown-cancelled context.

> **Backlog drain (was unspecified):** the **boot pass loops each phase until its expired set is empty** (paged at ~256), so a large backlog accumulated during downtime resolves promptly rather than at `limit`/3s. Steady-state ticks process one page per phase — ample for a 90s round's natural arrival rate, and self-catching-up because an unresolved page is still expired next tick.

> Sweep interval reconciled to **3s** (WS design's value over the deadline design's 5s): worst-case ~3s from buzzer to forfeit push; with the WS layer the outcome is broadcast the instant the sweep commits anyway. Tunable constant.

**Shared writer** — factor the score+rating+`SetMatchResult` tail of `persistResult` (`service.go:448-468`) into a helper whose signature **pins seat identity** so the judged and forfeit callers cannot map ratings to the wrong rows:

```go
// winner/loser identified by user_id, not seat index; helper writes rating_before/after
// AND users.rating for BOTH players, sets winner_player_id + judge_reason + resolution,
// all in qtx. Elo applied exactly once.
func (s *Service) writeFinalResult(ctx, qtx, matchID string, r finalResult) error
// finalResult{ winnerUserID, loserUserID, winnerBefore, winnerAfter,
//              loserBefore, loserAfter, reason, resolution }
```

`persistResult` (`resolution='judged'`) and the forfeit branch (`resolution='forfeit'`) both build a `finalResult` and call it, so Elo-writing — **including the `users.rating` update the ladder depends on** — lives in one place and cannot drift (see §2.7).

### 2.5 Concurrency

Every new write path reuses the codebase's single idiom: `Begin → GetMatchForUpdate (FOR UPDATE, now returning server_now) → recheck status under lock → act → Commit → publish`. No new primitive.

- **Submit vs. sweeper on the same row** — both take the row lock; the first commits, the second's `resolveExpiry` recheck sees `status != 'drawing'` and returns `outcomeNone` (no-op, no double publish). Same guarantee `persistResult` already relies on.
- **Two sweeper ticks / future horizontal scale** — batch selects use `FOR UPDATE SKIP LOCKED` (like `FindOpenMatchToJoin`), so concurrent sweeps partition the set.
- **Boot-time sweep** — `ListExpiredDrawingMatches` is deadline-based, not tick-based, and drains to empty, so the first `sweepOnce` recovers every deadline missed while the process was down (including rows backfilled by the migration).
- **Boundary** — `!serverNow.Before(deadline)` i.e. `serverNow >= deadline` = expired, evaluated on the DB clock. Pinned in a table-test.

### 2.6 Adjacent sweeps (bundled, same vehicle)

**Stuck-judging watchdog** — `sweepStuckJudging` re-locks `judging` rows whose `judging_started_at` is older than the stale interval, calls `SetMatchJudging` again (bumping `judge_attempts` and re-stamping `judging_started_at`), and re-fires `go judgeMatch(id)`. Safe to fire — `persistResult` re-locks and rechecks `status=='judging'` before writing, and `runJudging` re-renders persisted docs (idempotent output).

Two protections against firing on a merely-slow (not crashed) judge:
- **Staleness is measured against `judging_started_at`, set at the start of each attempt** — not `updated_at`, which is only bumped at the `→judging` transition. A legitimate render+ML round-trip no longer looks stuck simply because it's slow.
- **`stale_interval` is set well above observed p99 judge latency** (constant, tune against telemetry) so the watchdog is a crash/hang recovery, not a competitor to a healthy in-flight attempt. Even so, `judge_attempts` caps retries (default 3) to bound cost if the judge is genuinely wedged.

After the cap, both-submitted matches must not spin forever with no player-visible terminal state (a spinner showing `{ready:false}` indefinitely is a UX dead end). The interim affordance — a client-side "taking longer than expected" after a bound — is decided; the **exact terminal fallback** (a new `errored` terminal vs. forcing `abandoned` with no Elo) is a product/ladder call in §5 Q5.

**Open-match reaper** — `sweepStaleOpen` sweeps `open` matches older than a TTL to `abandoned` (via `SetMatchAbandoned`) so a solo creator who navigates away doesn't leave a ghost that `FindOpenMatchToJoin` later pairs a fresh player against (who would then "win" by forfeit/abandon over someone long gone). The mechanism is decided; the TTL length and the matchmaking UX around it (a "searching…" state, immediate cancel) are §5 Q9.

### 2.7 Forfeit / Elo semantics

**Solo-submitter (n == 1) — the judge does NOT run.**

- `afterSub, afterForf := computeElo(submitterRating, forfeiterRating, 1.0)` — the *same* `elo.go` function `runJudging` calls (`service.go:400`), `eloK = 32`, just a hardcoded `sa = scoreWin (1.0)` instead of a judge-derived score. All three source lenses converged here independently.
- **Winner row:** `rating_before/after` from `computeElo`, `drawing_id` = the submission. **Forfeiter row:** ratings from the same call, `drawing_id` stays null. `writeFinalResult` maps these by `user_id` (seat-independent), and a table test asserts the **forfeiter, not the submitter**, gets the loss row.
- **`writeFinalResult` updates `users.rating` for both players**, not only `match_players.rating_after` — otherwise forfeit Elo would never reach the ladder. This is the shared tail extracted from `persistResult`, verified by test for both callers.
- **`match_players.score` (the judge similarity score) is left null for both.** *Reconciliation:* the deadline design proposed writing `1.0/0.0` here; the WS design left it null. Null wins — `score` is the ML judge's similarity output, and no judge ran, so there is nothing to display. The S-value (1/0) lives in the Elo math, not in this column. The frontend reads `resolution === 'forfeit'` to branch its copy, never the score.
- **`matches`:** `winner_player_id = submitter`, `judge_reason = "opponent forfeited: no submission before the deadline"` (human prose; client never parses it), `resolution = 'forfeit'`, `status = 'done'` — all via `writeFinalResult` in one tx (Elo applied exactly once, atomically).

**Both submitted (n == 2)** → normal `judging` + existing `runJudging`, `resolution = 'judged'`.

**Nobody submitted (n == 0)** → `abandoned`, `winner_player_id` null, **no Elo writes at all**, no judge. Already fully wired on the client.

**Forfeit reveal:** `judgedImageUrl` stays null (as for *all* results today — object storage isn't built). When the membership-gated reveal lands, the result DTO's per-player `drawingId` already suffices: the winner's is present (client renders it via the existing gated endpoint), the forfeiter's is null (frontend shows an "opponent forfeited — no submission" placeholder). No server render needed for the forfeit case.

### 2.8 API / DTO changes (additive, mirror Go ⇄ TS in lockstep)

`handler.go` ⇄ `apps/web/src/core/api/matches.ts`:

- `matchDTO` / `Match`: `+ drawingDeadline: string | null` (RFC3339Nano, null while `open`), `+ serverTime: string` (RFC3339Nano, always present — `time.Now().UTC().Format(RFC3339Nano)` at response build; lets the client correct clock skew). **Both timestamps use the same format (RFC3339Nano)** to avoid subtle parse/format drift between tests and transports.
- `submitMatch` / `SubmitMatch` (202 body): same `drawingDeadline` + `serverTime` pair.
- **Submit 409-on-expiry:** `ErrRoundExpired` → HTTP 409 `{status, reason: "round_expired"}` (machine-readable, distinct from free-text `judge_reason`) so the client special-cases it without a generic error toast.
- `resultDone` / `MatchResultDone`: `+ resolution: 'judged' | 'forfeit'`. Non-null on every completed match because the migration backfills historical `done` rows to `'judged'`. `resultPlayerDTO` unchanged (Elo fields already apply; `score` may now be null on forfeit).

> This is match-DTO drift, not the frozen document contract — Go struct ↔ TS interface parity is required, but there is no TS/Go validator parity work.

### 2.9 Frontend (`PlayView.vue`)

- **Delete `ROUND_SECONDS = 90` and `startCountdown`.** Recompute each tick: `offsetMs = Date.parse(serverTime) - Date.now()`; `remainingMs = Date.parse(drawingDeadline) - (Date.now() + offsetMs)`. Re-anchoring every poll/frame removes drift structurally and fixes a **real existing bug** — today each client's 90s window starts whenever *it* first saw `drawing`, so the two players disagree by several poll cycles. Both now share one server instant. This closes the standing `TODO(play-api)`.
- **Auto-submit margin is a fixed constant, decoupled from poll cadence.** Fire at `deadline - AUTO_SUBMIT_MARGIN_MS` where `AUTO_SUBMIT_MARGIN_MS` is a constant (~3000ms), **not** a multiple of the poll interval — because the poll interval is now variable (§3.7 demotes it to ~15s under WS). A margin defined as `2 * POLL_MS` would silently steal 30s of drawing time the moment WS connects. Keep `POLL_MS = 2000` immutable and demote via a separate `pollCadence` variable. A submit landing after the server cutoff is a 409 that would self-inflict a forfeit loss; auto-submit is non-authoritative UX.
- **Handle 409 `round_expired`** in the submit mutation: switch `phase` to `judging`/`done` and resume polling instead of an error toast.
- **Monotonic phase (poll ↔ WS ordering).** `applyRoster`/`applyResult` guard against regression: once a **terminal** phase (`done`/`abandoned`) is applied, a later, slower in-flight poll returning `{ready:false}` is dropped rather than knocking the UI back to `judging`. `applyResult` is idempotent (re-applying the same verdict is a no-op), since both the WS `result` frame and the reconciliation poll can deliver it.
- **Result UI:** branch on `resolution === 'forfeit'` for "opponent forfeited — you win" vs. the normal score comparison. `status === 'abandoned'` is already wired in both poll arms. Forfeit flows through the existing `applyResult` path — no new phase.

---

## 3. WS-Realtime subsystem (`internal/ws`)

### 3.1 Overview & posture

A single-process, in-memory hub (`coder/websocket`) pushing match-room state to both duelists in real time — **while Postgres stays the sole source of truth and the REST poll loop remains a throttled fallback**. The hub owns zero authoritative state and never mediates a mutation: submit stays HTTP POST, transitions stay in `internal/game` under the row-lock idiom, and the hub only fans out read-only snapshots of things another subsystem already committed. It consumes the deadline subsystem's outputs — it does not reimplement them.

**Load-bearing invariants:**
- **Actor-model hub:** exactly one goroutine owns `rooms` with **no mutex** (map touched only inside its `select` loop). Connections are dumb read/write pumps.
- **Non-blocking-send-or-kill fan-out:** a slow/stalled client is force-closed, never waited on — one slow socket must never stall fan-out to every room in the process. Safe because every frame is superseded by a later full `match_state`; a dropped frame means staler info until the next, never corruption.
- **Commit-first, notify-after** everywhere: a publish never fires for an unpersisted transition, and a publish failure never rolls back a committed outcome (the same ordering `judgeMatch` already uses post-commit).
- **Per-viewer rendering, not marshal-once:** frames that can carry a viewer-dependent field are built **per recipient** (§3.6). Fan-out marshals once only for frames that are identical for both duelists.

### 3.2 The `game → ws` seam

One interface, injected like `render.Renderer` / `judge.Judge`, default `NopPublisher` so `internal/game` never imports `ws` concrete types and stays unit-testable and cycle-free:

```go
// internal/ws/events.go
type Publisher interface {
    // PublishOutcome/PublishState carry only the matchID + a tag; the hub itself
    // rebuilds any viewer-dependent DTO per connection (it holds the game.Service read seam).
    Publish(matchID string, ev Event)
}
```

`Hub.Publish` is a **non-blocking send into the hub's own buffered `publish` channel** (cap ~64); if full (pathological — hub loop wedged), drop + `slog`, never block a committed path.

**Publish is driven by committed outcome, from every `resolveExpiry` caller uniformly** — not from a fixed hand-listed set of call sites. The mapping:

| Committed transition (source) | Frame(s) |
|---|---|
| `CreateOrJoin` join branch (`open→drawing`) | `match_state` (full per-viewer snapshot; carries the deadline) |
| `Submit`, non-last submitter | `opponent_submitted` (`{userId}`, room-broadcast) |
| `Submit`, last submitter (`→judging`) | `opponent_submitted` then `judging` |
| `writeFinalResult` (judged, via `persistResult`) | `result` (`resolution:"judged"`, per-viewer) |
| `resolveExpiry` → `outcomeForfeit` (**sweep _or_ Submit late-path**) | `result` (`resolution:"forfeit"`, per-viewer) |
| `resolveExpiry` → `outcomeAbandoned` (sweep) | `abandoned` |
| connection register / unregister | `opponent_connected` / `opponent_disconnected` (presence, §3.5) |

The forfeit `result` is published from **both** the sweep and the `Submit` defense-in-depth path, because both call `resolveExpiry` and both run the uniform post-commit `publishOutcome` tail (§2.4). This closes the "winner not notified when the loser's own late submit triggers the forfeit" gap.

### 3.3 Package shape

```
internal/ws/
  hub.go     // Hub: single owning goroutine + register/unregister/publish channels
  room.go    // room: plain struct, conns keyed (userID → set), no own goroutine
  conn.go    // client: coder/websocket wrapper, read + write pumps
  events.go  // Event type, Publisher interface, JSON envelope
  handler.go // GET /api/matches/{id}/ws upgrade handler
```

- **Hub** — one goroutine (`Hub.Run(ctx)`), owns `rooms map[uuid.UUID]*room` mutex-free. On `unregister` emptying a room, `delete` inline (no reaper). On `publish`, look up the room and hand it the transition; if no room, **drop silently** (REST is still authoritative).
- **`room`** — plain struct, keyed `(userID → set of *client)` so **duplicate tabs/devices** are allowed. For a frame that is identical for both viewers (`opponent_submitted`, `judging`, `abandoned`, presence), `broadcast` marshals once and non-blocking-sends to every client. For a **per-viewer** frame (`match_state`, `result`), the room builds the DTO **once per distinct `userID`** via `game.Service`'s viewer-scoped read and sends each user their own bytes (§3.6). On full buffer, `forceClose` that client.
- **`client`** — two goroutines: `readPump` (drains frames only to detect close + service ping/pong; the client sends no application payloads) and `writePump` (the *only* socket writer, as `coder/websocket` requires). Each wrapped in `defer recover()` → logs + closes only that connection.
- **Per-user-per-match connection cap (v1).** On register, if the `(userID)` set for this room already holds `wsMaxConnsPerUser` (default 5) clients, the **oldest is force-closed** before admitting the new one. Authenticated ≠ unbounded: without this, one valid member can open thousands of sockets (2 goroutines + buffers each) via a reconnect loop. Cheap, and it belongs in v1 (see §5 Q7).

No table for the hub; connection/room state is purely in-memory (ARCHITECTURE §9's multi-instance trigger is not fired).

### 3.4 Upgrade & connection lifecycle — `GET /api/matches/{id}/ws`

1. Behind the **existing `RequireAuth`** — same `jp_session` cookie read + `parseToken` HS256 verify as REST (browsers auto-attach the cookie to the handshake GET; JS can't set headers on a WS handshake, so cookie auth is the only mechanism). Auth failure → plain **HTTP 401 before `Accept`**.
2. **Membership check before `Accept`:** caller must be a `match_players` row for this match, else plain **HTTP 404** (hidden ownership miss, never 403).
3. **Strict same-origin `OriginPatterns`** on `websocket.Accept`, never `InsecureSkipVerify` — WS handshakes bypass CORS preflight; without this a cross-site page could open a socket with the victim's auto-attached cookie (the WS analogue of CSRF).
4. **Session-expiry close:** at `Accept`, read the JWT `exp` and arm `time.AfterFunc(exp)` to close with **4001** — nothing re-validates the cookie mid-connection, and a tab can idle in `open` a long time.
5. On connect, immediately send a **per-viewer full `match_state` snapshot** built from the *same* `game.Service` method (scoped to the connecting `userID`) the REST `GET /matches/{id}` handler uses — one code path, two transports. Reconnect is trivially correct with **no replay buffer**.

### 3.5 Frame protocol

Server→client (JSON `{type, ...}`):

| type | payload | built | fires when |
|---|---|---|---|
| `match_state` | full `matchDTO` (+ `drawingDeadline`, `serverTime`) | **per viewer** | connect/reconnect; roster change |
| `opponent_submitted` | `{userId}` — flag only | shared | any player's `Submit` commits (room-broadcast; see below) |
| `judging` | `{}` | shared | last submit flips to `judging` |
| `result` | `resultDone` DTO (+ `resolution`) | **per viewer** | `persistResult` **or** any forfeit `resolveExpiry` commits |
| `abandoned` | `{}` | shared | sweep writes `abandoned` |
| `opponent_connected` / `opponent_disconnected` | `{userId}` | shared | a client for that `userId` registers / the last one unregisters |
| `pong` | `{}` | shared | reply to client `ping` |

Client→server: `{"type":"ping"}` **only** — submit stays HTTP POST; document data never crosses this channel.

- **`opponent_submitted` is room-broadcast, not opponent-only** — including to the submitter's own other tabs. The frame carries `{userId}`; **clients disambiguate self vs. opponent by comparing `userId` to their own** and ignore their own action. Broadcasting to self keeps the fan-out dumb and lets a second tab of the submitter stay consistent.
- **Presence** is `opponent_connected`/`opponent_disconnected`, emitted when a `userId`'s client set becomes non-empty / empty in the room. "Opponent is drawing" in the UI means precisely "opponent has a live socket" — this design does **not** stream strokes; presence is coarse liveness, best-effort (§3.7). The earlier presence UX claim is now backed by real frames rather than promised without a wire representation.
- **The deadline needs no recurring push:** it is one absolute timestamp, stamped once, riding in the `match_state` frame (and the REST response). The client computes its own countdown from `drawingDeadline` reconciled against `serverTime`.

### 3.6 Enforcing GAME.md §4.2 visibility — runtime redaction, per recipient

`match_state.players[].drawingId` **is** a real, viewer-dependent field: `buildMatchDTO` redacts it at runtime (`handler.go:106` — a player sees their own `drawingId`, and the opponent's only once `status==done`). The visibility guarantee is therefore **runtime per-viewer redaction**, not a structural "the type has no such field" claim.

Consequences, corrected from the source WS design:
- **`match_state` and `result` are built per recipient**, calling `buildMatchDTO(view, thatUserID)` once per distinct `userID` in the room. A marshal-once broadcast is **not** used for these frames — the two duelists legitimately need different bytes (each sees own `drawingId`, opponent's withheld until `done`). Marshal-once would either leak the opponent's `drawingId` mid-round or strip it for everyone (breaking self-view).
- `internal/ws` does not import `internal/document` or any drawings-query code; it obtains DTOs only through the same viewer-scoped `game.Service` read the REST handler uses. The hub has no path to canvas bytes; the only ids it ever emits are the ones REST would emit to that same viewer at that same instant.
- **Test (reframed):** assert that a mid-round (`status != done`) `match_state` built for viewer A contains A's `drawingId` and **not** B's — i.e. the per-viewer build matches REST redaction exactly. (The old "no frame field name matches `drawing|doc|canvas|image`" reflection test is dropped: it would false-fail on the legitimate `drawingId` field, and it asserts a structural guarantee that isn't the real one.)

Defense-in-depth note: even a mid-round `drawingId` leak is not *yet* directly exploitable, because `PlayerDrawing` gates the actual pixels on `status==done`. But exposing the id violates the DTO contract and becomes a live hole the moment any membership-only reveal ships, so the per-viewer build is required now, not later.

### 3.7 Frontend (`apps/web`)

- `core/api/matches.ts`: add `drawingDeadline`/`serverTime` to `Match`, `resolution` to `MatchResultDone`; add a typed `openMatchSocket(matchId)` wrapper (native `WebSocket`, same-origin cookie auto-sent) with a discriminated-union frame type + dispatch helper.
- `PlayView.vue`:
  - WS frames dispatch into the **same `applyRoster`/`applyResult`** functions the poll loop already calls — handlers are thin adapters, not new state machinery. Those functions are **monotonic and idempotent** (§2.9), so a WS `result` and a slower reconciliation poll can't fight.
  - **Poll loop demoted, never removed, via a separate cadence variable:** `POLL_MS` stays `2000` (and still feeds nothing timing-critical); a `pollCadence` ref starts at `POLL_MS`, moves to a slow reconciliation cadence (~15s) on WS `onopen`, and snaps back to `POLL_MS` immediately on `onclose`/`onerror`. The auto-submit margin (§2.9) is a fixed constant and does **not** read `pollCadence`.
  - The submit-race 409 (`round_expired`) transitions to the pushed terminal state, not a hard error.
  - Small honest **"reconnecting…"** affordance — degraded-to-poll-only should be visible, since presence quietly stops otherwise.
  - `presence` / "opponent is connected" is driven by `opponent_connected`/`opponent_disconnected`, is **best-effort, first-dropped under backpressure**, and is never load-bearing for correctness.

### 3.8 Concurrency & failure modes

- No mutex on room state (actor-model ownership) eliminates a class of race bugs.
- Non-blocking-or-kill fan-out; a dropped client falls back to REST + reconnect.
- Per-user-per-match connection cap (§3.3) bounds a single member's socket footprint; oldest-closed-on-exceed also breaks a runaway reconnect loop.
- Poll↔WS ordering is made safe on the client (monotonic terminal phase, idempotent `applyResult`) rather than by sequencing on the wire; frames carry no version because the terminal-phase guard is sufficient for a two-transport, two-player room.
- Hub crash/restart = drop all connections; clients reconnect via backoff and get a fresh snapshot. No recovery code, because the hub is never authoritative — a mid-match restart is a visible "countdowns stutter, then resync," flagged to product.

---

## 4. How they compose

### 4.1 The dependency

```
   internal/game (deadline subsystem)                internal/ws (realtime)
   ─────────────────────────────────                ──────────────────────
   stamps drawing_deadline  ───────────────────────▶  match_state frame (per viewer)
   resolveExpiry → done/forfeit/abandoned  ────────▶  result / abandoned frames
   Submit commits          ────────────────────────▶  opponent_submitted / judging frames
        (Postgres = source of truth)                        (delivery only)
```

The deadline subsystem **produces the facts**; the WS layer **delivers them faster**. Concretely:

- The **deadline instant** the hub broadcasts (`match_state.drawingDeadline`) exists only because the deadline subsystem stamps it. Without it, `match_state` has no timer to carry and the client is back to its drifting client-only guess.
- The **`result` and `abandoned` frames** are literally the uniform post-commit tail on every `resolveExpiry` caller. Without server-authoritative resolution, the hub would have nothing truthful to push at the buzzer — and a WS-only trigger (resolve when a socket's room timer fires) is unsafe: a match with no live connection would never resolve. The sweeper is the authoritative enforcement; the hub is a notifier bolted onto its committed writes.
- The **`Publisher` seam + `NopPublisher` default** means the deadline subsystem is fully functional and fully tested with zero WS code present. The hub is strictly additive.

**What each unlocks:**
- Deadline subsystem alone: correct forfeit/abandon semantics, real Elo on timeout (reaching `users.rating`), a shared server clock that fixes the two-players-disagree timer bug, no stuck matches, no immortal migration-caught rows, no ghost open matches, crash-mid-judge recovery — all over the *existing* 2s poll loop, **no new `go.mod` dependency**.
- WS layer on top: sub-second presence and verdict delivery, "opponent connected/submitted" liveness, and the demoted poll loop as fallback — pure latency/UX, adding `coder/websocket`.

### 4.2 Recommended implementation order

**Build the Round-Deadline subsystem first; the WS-Realtime layer second.**

Rationale:
1. **Correctness before latency.** The deadline subsystem fixes real bugs (drifting timer, matches that never resolve, forfeit unimplemented, Elo that never reaches the ladder) and closes security-relevant gaps (late-submit stealing a forfeit; enforcement on the DB clock). The WS layer improves *how fast* the user sees an outcome the deadline subsystem must first produce correctly.
2. **The WS layer cannot be correct without it.** Its `result`/`abandoned`/deadline frames are hooks on the deadline subsystem's committed writes. Building WS first would force a throwaway stub for exactly the transitions this design says must be authoritative.
3. **Smaller blast radius, no new dependency.** The deadline subsystem is a migration + sqlc + one shared routine + three sweeps + additive DTO/frontend changes, all on the existing lock idiom. It ships and de-risks independently. WS adds a new dependency (deferred to "Phase 3 back-half" per CLAUDE.md) and the process's only concurrent-connection surface — worth landing on a proven-correct base.
4. **The seam is already there.** Land the `Publisher` interface with the deadline subsystem (default `NopPublisher`), so step two is "implement the hub behind an existing seam," not "retrofit publish points."

**Rough relative sizing:**

| Subsystem | Size | Composition |
|---|---|---|
| Round-Deadline | **M** | migration `00004` (+ backfills, 3 partial indexes) + sqlc regen; `resolveExpiry` + `writeFinalResult` refactor + sweeper (expired / stuck-judging / stale-open); DB-clock `Submit` 409 path; DTO fields (`drawingDeadline`/`serverTime`/`resolution`); `PlayView.vue` timer rework + monotonic phase; ~16 tests. No new dep. |
| WS-Realtime | **L** | new `internal/ws` package (hub concurrency, upgrade handler, security posture, presence + per-viewer frames, conn cap); `coder/websocket` added to `go.mod`; publish through the uniform outcome tail; frontend socket wrapper + poll demotion + reconnect UX; race/leak/visibility tests. |

A natural seam between them: land the deadline subsystem and the `Publisher` interface as one branch (backend + frontend timer), verify the async loop end-to-end over the poll loop, then open the WS branch. Per the repo's fan-out convention, backend (`server/`) and frontend (`apps/web`) share no files and can proceed on parallel branches within each subsystem, serialized through the orchestrator for the shared `00004` migration and the `matches.ts` DTO edits.

---

## 5. Open product questions

Engineering is decided above; these are genuine product/ladder calls, defaults in **bold**.

> **DECIDED 2026-07-11 (owner): all defaults below accepted as-is** — full-K forfeit Elo (#1); reject late submit, keep a `grace = 0` constant for later (#2); min-stroke anti-cheat deferred as a separate submit-validation feature (#3); no explicit abandon endpoint yet (#4); stuck-judging cap = 3, `errored` terminal added in a later slice (#5); sweep 3s (#6); v1 per-user WS cap only (#7); multi-instance out of scope (#8); open-match reaper TTL ~10 min + a "searching…"/cancel UX (#9). These are now the plan, not open questions.

1. **Forfeit Elo discount.** **Default: full `eloK = 32`, forfeit win == judged decisive win** (`sa = 1.0`). A ladder might discount an "opponent went AFK" win (half-K, or `sa = 0.75`) so it rewards less than out-drawing someone. Nothing locked requires it; the change is isolated to the forfeit branch's `sa`/K feed into the same `computeElo`.
2. **Grace window on late submit.** **Default: reject, no grace** (409 `round_expired`) — strictest reading of "both submitted *at* the deadline judges normally." A soft 1–2s grace to absorb render+network latency is a legitimate fairness alternative; the fixed-margin auto-submit largely mitigates the harm either way. Revisit if telemetry shows real submissions lost to latency.
3. **Anti-cheat on forfeit.** A player can submit a blank doodle the instant their opponent no-shows and bank a full forfeit win — Elo-correct by the locked rules. Unknown whether a min-stroke-count check exists at submit. Flagging, not assuming; out of scope for these subsystems.
4. **Explicit `POST /matches/{id}/abandon`.** The sweep handles *timeout*; a player wanting to *quit mid-round immediately* (not wait out 90s) still has no path. Cheap to add later as a thin handler running the same `resolveExpiry` branch early. Deferred — flag if a "quit" button is desired UX. (API.md §8 spec's it as optional and unimplemented.)
5. **Stuck-judging retry-cap policy AND its player-facing terminal.** **Default: cap at 3 attempts, then leave stuck-and-logged** (both players submitted → a judge-infra failure worth an alert, not silent data loss) — *plus* a decided interim client "taking longer than expected" affordance so the spinner isn't silent. The open call is the **terminal fallback after the cap**: a new `errored` terminal state (client leaves the spinner, no Elo) vs. forcing `abandoned` (reuses wired client handling but mislabels a both-submitted match) vs. an ops-only manual re-judge. Adjacent to the deadline subsystem; the sweeper is the vehicle, but the policy and the terminal labeling are product calls.
6. **Sweep interval = 3s.** Worst-case ~3s from deadline to the forfeit/abandon push (instant once WS delivers it). Fine for a 90s round; tighten only if the verdict must feel instantaneous at the buzzer.
7. **WS connection hardening beyond the v1 cap.** **Decided for v1: a per-user-per-match cap (default 5, oldest-closed-on-exceed).** Still open: a global connection semaphore, idle-timeout eviction, and per-IP limits — judged premature for JWT-gated, membership-404'd, two-player rooms, but revisit before live mode goes public (or the moment ARCHITECTURE §9's multi-instance trigger fires).
8. **Multi-instance hub.** Explicitly out of scope — the in-process hub is a known ceiling. When the trigger fires, the `Publisher` seam is the extraction point (swap in-process fan-out for Postgres `LISTEN/NOTIFY` or Redis behind the same interface) with no `internal/game` changes.
9. **Open-match TTL & matchmaking UX.** The **mechanism is decided** (a `created_at`-based reaper sweeps never-joined `open` matches to `abandoned`, so no ghost ambushes a later joiner). Open: the **TTL length** and the surrounding UX — whether a solo creator sees a "searching for an opponent…" state, an explicit cancel, or a silent expiry — and whether an expired-open match should be `abandoned` vs. hard-deleted. Product call; the sweep is a tunable constant either way.

---

*Files touched across both subsystems: `server/migrations/00004_round_deadline.sql` (new, with backfills + three partial indexes); `server/internal/game/{deadline.go, sweeper.go, service.go, handler.go, queries/matches.sql}`; `server/internal/ws/*` (new package); `server/cmd/server/main.go` (start sweeper + hub, mount WS route behind `RequireAuth`); `apps/web/src/core/api/matches.ts`; `apps/web/src/views/PlayView.vue`. Docs to update in lockstep: `docs/GAME.md` §3/§4.1/§9, `docs/API.md` §8/§9, `docs/ARCHITECTURE.md` §8, `docs/DECISIONS.md`. `coder/websocket` is added to `server/go.mod` only in the WS step.*