# Game — match lifecycle & product rules

> **The north star spec.** The AI-judged drawing duel (`/play`): two players draw the *same* prompt, the server renders authoritative rasters, the judge scores them, a winner (or tie) is recorded. This doc owns the **match lifecycle/state machine, the canonical game canvas size, prompt pinning, the ratings sketch, and tie handling**. It defers the judge contract wholesale to `docs/JUDGE.md` and the document/storage schema to `docs/DOCUMENT-FORMAT.md`.
>
> **Status:** Shipped (Phase 3, `docs/ROADMAP.md`). The async duel is **v1**; live realtime (§9) has now shipped as a delivery upgrade, without forking the lifecycle. Single-player practice (§10) shipped as a Phase 4 addition — a separate mode alongside the duel, not a fork of it. Companion: `docs/DECISIONS.md` (the "why"), `docs/ARCHITECTURE.md` (§7 data model, §8 async-first), `docs/JUDGE.md` (scoring contract), `docs/API.md` (routes, error shape, DoS caps). When this disagrees with those for what it owns, this doc wins; for what it defers, they win.

## 1. Scope & ownership

This doc **owns**:
- the match **state machine** (`open | drawing | judging | done | abandoned`) and every transition (§3, §4);
- the **daily AI-call budget** gating every new match, every practice run, every `/draw` AI guess *and* every AI-assist call (§4.3);
- the **canonical game canvas** = square **1080×1080** (§2);
- how a **prompt** is pinned per match (§5);
- the **trust boundary** for game submissions (§6);
- the **ratings sketch** (Elo-style), **tie** handling, and the leaderboard (§8).
- **single-player practice** — why it is not a match, and its place in the budget and the ratings (§10).

The budget in §4.3 is no longer only the game's: it also bounds AI assist (`docs/ASSIST.md`), the `/draw` guess (`docs/API.md` §13) and any future AI feature. It stays documented here because this is where it was first derived and where its interaction with the match lifecycle is decided; the code lives in its own module, `server/internal/aibudget`.

This doc **defers**:
- the **judge contract** — `Result` shape, positional `winner` type, tie rule, raster size (1024×1024), background — entirely to **`docs/JUDGE.md`**. We only consume it.
- the practice **`Critic`** seam and the `/draw` **`Guesser`** seam — entirely to **`docs/JUDGE.md`** (§8.2 and §8.3 there). This doc only says a practice run spends the same budget and pins its own semantics (§10); for the guess it says only that it spends the same budget (§4.3) — the feature itself is not a game mode and lives in `docs/API.md` §13.
- the **`drawings` table** to **`docs/DOCUMENT-FORMAT.md` §7**; the **`users` / `prompts` / `matches` / `match_players`** columns to **`docs/ARCHITECTURE.md` §7** (this doc tightens their *semantics*, never re-declares the column names); the **`practice_runs`** table columns to its migration comment (`server/migrations/00006_practice_runs.sql`) — no other doc re-declares them.
- **routes, auth cookie (`jp_session`), error envelope, DoS caps** to **`docs/API.md`**.

## 2. The canonical game canvas

**Game canvas = square `1080 × 1080` logical units** (`GAME_CANVAS`). Both duelists draw on this exact size; their submitted documents carry `width = height = 1080`.

- **Why square, why 1080.** The judge frame is a square **1024×1024** (`JUDGE_FRAME`, owned by `docs/JUDGE.md`). Letterbox bars carry no drawing but still count as judged pixels and skew similarity scores; a square game canvas fit (contain) into a square judge frame produces **zero letterbox** (`DOCUMENT-FORMAT.md` §2 scope note). 1080 is a clean, familiar editing size that downscales cleanly to the 1024 judge frame.
- **This pins only the game.** The general free-draw default stays `1920×1080` (`DOCUMENT-FORMAT.md` §2). Only `/play` constrains the canvas to square; `/draw` does not.
- **Enforced at submit.** The submit path rejects a game document whose `width`/`height` ≠ `GAME_CANVAS` (`validation_failed`, per `docs/API.md`). The canonical raster handed to the judge is rendered server-side regardless (§6) — the size check just keeps both players on one honest space.

## 3. Match states

Five states on `matches.status` (`ARCHITECTURE.md` §7), exact enum:

| State | Meaning |
|---|---|
| `open` | Match created, prompt pinned; waiting for the roster to fill (both `match_players` rows present and ready). |
| `drawing` | Both players are in; each draws the same prompt **independently on their own canvas**. |
| `judging` | Both submitted; server is rendering authoritative rasters and awaiting the judge. |
| `done` | Judge returned, **or** the round deadline passed with exactly one submitter (forfeit), **or** judging exhausted its retries with no verdict (aborted); result (scores, winner-or-tie, reason, and how it was decided) recorded; ratings applied — except on an abort, which applies none. Terminal. |
| `abandoned` | Match ended without a result (timeout / a player never submitted / cancelled). Terminal. |

Only `done` and `abandoned` are terminal. `winner_player_id` is meaningful only in `done` (and may be `null` there — a tie, §8, or an abort, which is **not** a tie). A `done` match also carries `resolution` (`judged` | `forfeit` | `aborted`) — how it was decided (§4.1).

## 4. The async duel — flow & transitions

The full loop, v1 (**HTTP only**; no realtime required — `ARCHITECTURE.md` §8):

```
create match ──▶ both draw the SAME prompt ──▶ submit ──▶ server renders
(pin 1 prompt)   (square 1080², own canvas)   (vector    authoritative
                                                doc only)  1024² PNGs
                                                              │
                                                              ▼
                                                           judge ──▶ result
                                                        (positional   (reveal BOTH
                                                         A/B/tie)       canvases +
                                                                        scores +
                                                                        reason +
                                                                        winner)
```

### 4.1 Transitions (what triggers each)

Since `feat/round-deadline`, the `drawing` round carries a **server-authoritative deadline**: `matches.drawing_deadline` is stamped `now() + 90s` on Postgres' own clock the instant `open → drawing` fires, enforced both by a background sweeper and defensively on every `submit` — so a round resolves even when nobody is polling.

| From → To | Trigger | Server actions |
|---|---|---|
| *(none)* → `open` | A player **creates** a match. | Insert `matches` row; **pin exactly one prompt** (§5) → `matches.prompt_id`; set `mode = 'async'`, `status = 'open'`; insert the creator's `match_players` row. |
| `open` → `drawing` | The **roster fills** (second player joins a 1v1). | Insert the second `match_players` row; flip `status = 'drawing'` and stamp `drawing_deadline = now() + 90s`. The prompt is now revealed to both. |
| `drawing` → `judging` | The **last** outstanding player **submits** (before the deadline). | Persist each submission as a `drawings` row (`match_id` set), stamp `match_players.drawing_id` + `submitted_at`. When all roster slots have a submission, flip `status = 'judging'` and kick off rendering. |
| `judging` → `done` | The **judge returns**. | Render authoritative PNGs (§6) → call the judge (`docs/JUDGE.md`) → write `match_players.score`, map positional `winner` → `matches.winner_player_id` (null on tie, §7.1), store `matches.judge_reason` and `resolution = 'judged'`; apply ratings (§8); flip `status = 'done'`. |
| `drawing` → `done` (forfeit) | The **round deadline passes** with exactly **one** submitter. | The submitter **wins by default** — the judge does *not* run (there is only one image). Apply Elo via the same `computeElo` with the submitter's score fixed at `1.0` (full `K = 32`, same weight as a decisive judged win, §8); `match_players.score` stays `null` for both (no judge similarity to record); `matches.resolution = 'forfeit'`, `judge_reason` a fixed human-readable string. |
| `judging` → `done` (aborted) | Judging **exhausted its retries** (`judge_attempts` hit the cap and the attempt went stale) — a wedged judge or a dead render worker. | Flip `status = 'done'` with `winner_player_id = null`, `resolution = 'aborted'`, `judge_reason` a fixed player-facing string; **no scores, no rating change** (no judge ever ran). Without this the match would sit in `judging` forever, since the re-fire watchdog stops at the cap. |
| `drawing` → `abandoned` | The **round deadline passes** with **zero** submitters. | Flip `status = 'abandoned'`; no scores, no rating change — nobody drew. |
| `open` → `abandoned` | An **open match sits unjoined past a TTL** (~10 min). | A background reaper sweeps stale `open` matches to `abandoned` so a ghost match can't later pair a fresh joiner against a creator who's long gone. (**Explicit cancel** is still just the optional, unimplemented `API.md` §8 `POST …/abandon` — not a shipped trigger.) |

Notes:
- **`aborted` is a last resort, not a retry policy.** The stuck-judging watchdog re-fires a stale attempt up to the cap first (`DESIGN-PHASE3-LIVE.md` §2.6); only a match that burned every retry is closed out this way, and it is logged at error level as a judge-infrastructure failure worth an alert. The two sweeps are exact complements — below the cap the row is re-fired, at or past it the row is aborted — so no wedged match can fall between them (`DECISIONS.md` 2026-09-18).
- A **submit into a non-`drawing` match** (e.g. already `judging`/`done`/`abandoned`) is an **illegal transition** → `409 conflict` (`docs/API.md`). The state machine, not the client, gates this.
- The `drawing → judging` flip is **all-or-nothing on the roster**: one player submitting does not advance the match; it only stamps their slot. The match advances when the *last* slot is filled.
- **Idempotent submit:** re-submitting an already-stamped slot is rejected (`409`), so a double-tap can't overwrite a submission or re-trigger judging.
- **Late submit (deadline passed):** a submit landing at or after `drawing_deadline` is rejected — `409 conflict`, message `"round expired"` — and is **not** stamped, even though the match may still read `status: drawing` at that instant (the check runs on the same Postgres clock that stamped the deadline, so it can't be raced by host-clock skew). The round resolves to forfeit/abandoned as part of rejecting the late submit, if the background sweeper hasn't already gotten to it first.

### 4.2 Visibility rule

**During a round each player sees ONLY their own canvas.** No peeking at the opponent's in-progress (or finished) drawing while the match is live. **Both canvases are revealed together on the result screen** once `status = done` (`DECISIONS.md` "Game screen visibility"). The opponent's `drawing_id` / rendered raster is not exposed by any read endpoint until the match is `done`.

### 4.3 Daily AI-call budget

Every `POST /api/matches` call, **every `POST /api/practice` run** (§10), **every `POST /api/guess`** (`API.md` §13) **and every `POST /api/assist/ops`** (`ASSIST.md` §3.4) is gated on a **daily AI-call budget** before any expensive work happens — for a match, checked once before any of the three `CreateOrJoin` branches above run; for a practice run, checked once at the very top of `practice.Service.Run`, before the render or the critic is ever reached; for a guess, checked at the top of `guess.Service.Guess` too, second only to the "is a guesser even wired" test, so an unconfigured feature never consults a budget for a call it will never make; for an assist call, checked last of that handler's guards and immediately before the one line that costs money. All four ask the same `internal/aibudget` (`server/internal/aibudget/`), each holding it as a pair of plain funcs bound to its own **kind** at the composition root — never a second copy — so features that spend the same external quota are counted by **one rule**, never two that could drift. For a match this sits ahead of the matchmaking transaction, since these are advisory reads and that transaction holds row locks worth keeping short. It exists because the per-IP rate limiter (`API.md` §3.1) bounds request *rate*, not the resource actually at risk: an AI provider runs on a quota measured in requests per **day**, one duel / practice run / guess / assist call costs exactly one request, and the write tier alone (30/minute) lets a single IP threaten a whole day's quota within minutes (`DECISIONS.md` 2026-09-19, 2026-09-20).

**Counted over one ledger table, not derived from the domain tables** (`server/migrations/00007_ai_calls.sql`, `server/internal/db/queries/ai_calls.sql`). The budget used to be a union over whichever tables a feature happened to write — `matches` / `match_players` for a duel, `practice_runs` for a practice run — each arm carrying its own judgement about *which lifecycle column means a call was actually spent*. Two of the AI features cannot be counted that way at all: a `/draw` **guess** has no row anywhere, because the drawing may never be saved, and **assist** has no row either and was bounded only by an in-process token bucket that the host resets on every deploy and every wake from idle — so that ceiling had never actually held. Every spend now appends to `ai_calls`, in a uniform shape with no special cases: **one row per provider request** (`user_id` null, `provider` set) plus **one row per billed player** (`user_id` set, `provider` null). A duel writes **3** rows — one provider, two players — and a practice / guess / assist call writes **2**. That split is what lets one duel cost the provider a single request while costing two players a day's allowance each, with no `DISTINCT`, no divisor and no join; a check constraint (`ai_calls_bills_something`) rejects a row that bills neither.

Two independent halves, both must clear:

- **Per-user, and per KIND** (`aibudget.KindSpentError` → `429`, checked first): how many calls of *this kind* this player has spent inside the rolling window. Per-kind, not one shared pot across every AI feature: spending a day's duels no longer costs a player their assists, because the two are not substitutes for each other and one shared number could only ever be wrong for one of them. The honest cost: a single player's exposure on the Google-backed kinds rises from 20/day to 20 + 20 + 2 = **42**, against the same global 200 — so roughly five heavy players can reach the global ceiling, which is exactly what the global half is there to catch.
- **Global, and per PROVIDER** (`aibudget.ErrGlobalSpent` → `429`, checked second, only once the caller clears their own cap): how much of *one provider's* quota this service has spent inside the same window, across every kind. Per provider, never service-wide — Google running dry must not refuse an Anthropic-backed feature that still has quota. This is also the one refusal nobody outside can see, so it is the one the budget logs (`Warn`): every feature backed by that provider is off for **everyone** until the window rolls.

Per-user is checked first because it is the case that actually fires and it is the cheaper read; a player over their own cap is told exactly that, never anything about the service's remaining budget. The **global** refusal discloses nothing about the budget's size — that is operator information — while a per-kind refusal may name the player's *own* cap where it is small enough that they could have counted it themselves, which is why the guess message names its number and the others do not (`API.md` §8/§12/§13; the copy lives in one place, `server/internal/aibudget/http.go`).

**Spending is explicit, and it happens BEFORE the provider call** — never inferred afterwards from a lifecycle column:

- A **duel** is billed inside the matchmaking transaction, at the one site that causes a judge call: `open → drawing`, the join branch. **Both** players are billed for the single provider request, from the roster that transaction just read — nobody is drawn into a duel passively, both sides pressed play, and both get the round. The branches that do *not* start a round bill nothing, so "a match nobody ever joined costs the player nothing" is now true **by construction** rather than by a query's choice of anchor column.
- A **practice run** and a **guess** are both billed **between the render and the model call**, and an **assist** call immediately before the impl is invoked — all outside any transaction, deliberately. Never *after* the call: a critique or a guess that failed still spent the provider's quota, and a failure the budget cannot see is exactly what a broken impl drains it through — a transaction would roll the record back for the same reason and be just as wrong. But not *before* the render either, and that is the narrower half of the window: the render is our own subprocess and spends nobody's quota, so a renderer that fell over must not cost a player a scored drawing, or one of the two guesses they get for the day. Everything above that line is free to re-run; nothing below it is.

**Window:** rolling 24h, not a calendar day — no timezone to get wrong, and since both ceilings sit under the provider's own per-day quota, never exceeding either in *any* 24h also never exceeds it in whatever calendar day the provider counts. It is measured by **Postgres**, against the database's own `now()`, so the window cannot skew when the app clock and the DB clock disagree.

**Defaults:** **200** global *per provider* (`AI_DAILY_GLOBAL`, `config.DefaultAIDailyGlobal`), and per player per rolling day **duel 20, practice 20, guess 2, assist 40** (`aibudget.DefaultPerUser`), overridden per kind by `AI_DAILY_PER_USER="duel=20,guess=2"`. Duel and practice keep the old shared 20 because they are the same act with and without an opponent; guess is deliberately tiny — a novelty question about a drawing that may never be saved, otherwise the cheapest way to empty a provider quota — and assist is the loosest because it is a working tool, and a ceiling that interrupts a drawing session breaks the feature rather than bounding it. Every value is rejected below 1 at boot — there is no "unlimited" setting — and a per-user cap set *above* the global one is kept legal on purpose, as the idiom for "no real per-player limit, the global budget is the only ceiling." The pre-per-kind names still work, so a deployed environment needs no edit (`server/.env.example`): `JUDGE_DAILY_BUDGET` is a straight **alias** of `AI_DAILY_GLOBAL` (both set to *different* values is a boot error naming both), and `JUDGE_DAILY_PER_USER` seeds **only** `duel` and `practice` — the two kinds that shared it when it was named — so an operator's old `20` never quietly becomes 20 AI guesses a day.

**Enforced only for a kind whose impl is real.** A kind with no provider is never checked and never recorded; that is the same fact said once rather than a separate "enforced" flag, since you cannot spend a quota you have no provider for. Which provider backs which kind is resolved once, at the composition root (`aiPolicies`, `server/cmd/server/main.go`), from the mode switches: `JUDGE_MODE=gemini` puts duel, practice *and* guess on `google`; `JUDGE_MODE=http` puts the duel on `collaborator` (his quota, not ours, and still worth a ceiling under it) and leaves practice *and* guess unprovidered, since that mode has neither a critic nor a guesser at all and so makes no calls to owe quota for (`JUDGE.md` §8.2, §8.3); `ASSIST_MODE=anthropic` puts assist on `anthropic`. Every `fake` impl has no provider, so dev and CI never meet a ceiling. `guess` is now a wired feature rather than a reserved kind (`POST /api/guess`, `API.md` §13), which is what puts one player's Google-backed exposure at 42/day.

Both counts are **advisory, not transactional** — deliberately: they are reads with no lock, so two simultaneous callers can each read a count one below the ceiling and both pass. Serializing every match creation (or practice run, or assist call) on a budget row would cost more than the handful of calls a burst can overshoot by, and the ceilings sit *under* the external quota precisely so a small overshoot stays inside it. What the ledger changed is *which* inaccuracies remain:

- **A concurrent burst can overshoot** by roughly the number of checks in flight at once. That plain check-then-record race is now the only structural one — the old "the global count cannot see matches currently `drawing`" gap is **gone**, since a duel is recorded the instant its round starts rather than when it reaches `judging`, and so is the old "a practice run in flight is invisible until its row commits", since practice and assist write their rows before the provider call and outside any transaction.
- **The ledger bills the act, not each HTTP attempt.** One duel is one provider row no matter how many requests it really costs: `Judge.Score` makes up to 3 attempts (`JUDGE.md` §7), and a stuck-judging re-fire (§4.1) runs a whole second pass — all of it against that single row.
- **A duel's rows are invisible to a concurrent count until its transaction commits**, because they are written inside the matchmaking transaction. Milliseconds, and the narrow price of "no round, no judge call" being atomic.
- **A duel's creator is checked when they create and billed when someone joins**, up to the open-match TTL later (§4.1). A player who spends the rest of their duel allowance in the meantime can therefore be billed one duel over their own cap. Refusing at creation is still the humane place for the check: a player told "you are out of duels" before they pick up the brush has lost nothing, while the same message after two minutes of drawing would be worse than the bug it guards.
- **A failed call is still counted.** The rows are written before the provider is called and are never rolled back, so a provider outage burns allowance. That is the conservative direction on purpose — a failure that costs nothing is exactly the one that drains a quota unseen.

Rows are swept on a retention horizon of a week (`aibudget.RunSweeper`, hourly): nothing is ever *read* past the 24h window, but a week of history is what gives "why did the ceiling refuse me last Tuesday" an answer, at a few thousand rows.

## 5. Prompts

- **Source:** the `prompts` table (`ARCHITECTURE.md` §7) — `id`, `text`, `active bool default true`, `created_at`. Seeded server-side; `active = false` retires a prompt without deleting history that referenced it.
- **One prompt per match, pinned at creation.** Match creation selects a single active prompt and writes `matches.prompt_id`. **Both players draw that same prompt** — it is the shared target the judge scores similarity against. The prompt is fixed for the match's whole life (no re-roll mid-match).
- **Selection (v1):** random among `active = true`. (Curated/themed/difficulty-tiered selection is a later option — not v1.)
- **Reveal timing:** the prompt text is delivered to a player only once they're in the match and it has entered `drawing` (so a player can't pre-draw before the roster fills). Fairness: both players get the same prompt at effectively the same moment.

## 6. Trust boundary — authoritative server render

Game-critical, and inherited from `DOCUMENT-FORMAT.md` §10 / `ARCHITECTURE.md` §6:

- **The client submits the vector `document`, never a scored PNG.** A client-side thumbnail may ride along for instant UI, but it is **advisory only** — never fed to the judge, never scored.
- **The server renders the authoritative raster off the player's machine** from the submitted document, using the shared `packages/document` renderer (Node render worker — `ARCHITECTURE.md` §8/§9), producing the **square 1024×1024** judge frame with an **opaque (white) background** as pinned in `docs/JUDGE.md` / `DOCUMENT-FORMAT.md` §10 (`RenderOptions.background` overrides `doc.background`). This kills any "submit a doctored PNG" attack — the score is computed only over pixels the server itself produced from the validated document.
- **Validation happens first.** The submitted document runs the full Go validator at the write edge (`DOCUMENT-FORMAT.md` §7, DoS caps in `docs/API.md`) before it is ever rendered or judged. An invalid/oversized doc is rejected (`400` / `413`) and the slot is **not** stamped.

## 7. Data tables & the A/B → player mapping

Columns are owned elsewhere — this section pins only the **game semantics** over them.

- **`matches`** (`ARCHITECTURE.md` §7): `mode = 'async'` for v1; `status` per §3; `prompt_id` per §5; `winner_player_id` **nullable** (null = tie/undecided, §7.1, §8); `judge_reason` = the judge's `reason` string verbatim.
- **`match_players`** (join, **two rows per 1v1**, `ARCHITECTURE.md` §7): `drawing_id` → the player's submission (`drawings`, `DOCUMENT-FORMAT.md` §7); `score double precision` from the judge; `submitted_at`; `rating_before` / `rating_after` (§8). The two-row join is the primitive that later generalizes to teams/tournaments without reshaping `matches`.
- **`drawings`** — see **`DOCUMENT-FORMAT.md` §7**. A duel submission has `match_id` set and `owner_id` = the submitting player; ownership-scoped like every drawing (no IDOR — `docs/API.md`).

### 7.1 Mapping positional A/B to player ids

The judge speaks only in **positional** terms — it scores `imageA` vs `imageB` and returns `winner ∈ {"A","B","tie"}` (owned by `docs/JUDGE.md`); it has no notion of users. The **`game` module** owns the binding:

1. At judging, the module picks a stable ordering of the two `match_players` (e.g. by `submitted_at`, then `user_id` as tiebreak) and renders player-1's doc as **image A**, player-2's as **image B**.
2. It records each player's `score` (`scoreA` → player-A's `match_players.score`, `scoreB` → player-B's).
3. It maps the positional `winner` back to a concrete player: `"A"` → player-A's `user_id`, `"B"` → player-B's, **`"tie"` → `null`** → written to `matches.winner_player_id`.

The mapping lives **only** here; the judge never learns who is who, and the stored result is always in resolved-player terms.

## 8. Ratings (Elo-style sketch)

> **Scope:** a deliberately small Elo sketch so a result *moves the needle*. **Full ratings are Phase 4** (`ROADMAP.md`) — decay, provisional/placement handling, anti-abuse are out of scope here. This pins just enough to apply a rating delta on every `done` match.

- **Storage:** `users.rating int not null default 1200` (the live rating); `match_players.rating_before` / `rating_after` snapshot each player's rating around the match (audit + display). All three owned by `ARCHITECTURE.md` §7.
- **Model (standard Elo):** expected score for player P against opponent O
  `E_P = 1 / (1 + 10^((rating_O − rating_P) / 400))`.
  Actual score `S_P`: **win = 1, loss = 0, tie = 0.5** (shared/half points — `DECISIONS.md` "Ties are allowed").
  New rating: `rating_after = round(rating_before + K · (S_P − E_P))`.
- **K-factor:** **K = 32** for v1 (a single flat K — simple, responsive; tiered/provisional K is Phase 4).
- **Tie:** both players take `S = 0.5`; the deltas are equal-and-opposite only when ratings were equal, otherwise the lower-rated player gains and the higher-rated loses a little, as Elo intends. `matches.winner_player_id = null`.
- **Forfeit:** if the round deadline passes with exactly one submitter (§4.1), that player's actual score is fixed at `S = 1` fed into the *same* `computeElo` — full `K = 32`, exactly like a decisive judged win, no discount. The judge never runs (only one image exists), so `match_players.score` (the judge similarity) stays `null` for both players; the S-value lives only in the Elo math, not in that column. `matches.resolution = 'forfeit'` distinguishes it from `'judged'`.
- **Aborted:** a `done` match whose `resolution = 'aborted'` (judging exhausted its retries, §4.1) applies **no** rating change at all and leaves `match_players.score` / `rating_before` / `rating_after` null — no judge ran, so there is no outcome to rate. It is `done` only because it is terminal and has a reason to show; for every rating purpose it behaves like `abandoned`.
- **When applied:** exactly once, atomically, on the `judging → done` transition (a judged result) **or the `drawing → done` forfeit transition** (§4.1) — after the result is recorded. `rating_before` is captured before the update; `rating_after` after. An `abandoned` match — or an `aborted` one — applies **no** rating change.
- **Atomic ladder write:** the delta reaches `users.rating` via `rating = rating + delta` (`ApplyRatingDelta`, `RETURNING` the true post-value), **never** an absolute `SET`, so two matches seating the same player and resolving concurrently both land — the match row lock serializes per *match*, not per *user* (`docs/NOTES.md`, `DECISIONS.md` 2026-07-17). The delta *magnitude* is sized from a pre-match rating snapshot (a rating period — standard Elo), while `rating_before`/`rating_after` are derived from that atomic write's `RETURNING` so the snapshot always agrees with the ladder.
- **Outcome from the judge, not the score gap:** win/loss/tie is taken from the judge's `winner` field (mapped per §7.1), not by comparing `scoreA`/`scoreB` ourselves — the judge owns the verdict, including whether a near-equal pair is a tie. (A forfeit has no judge outcome to take — the winner is simply the submitter.)
- **Leaderboard (Phase 4, shipped — `ListTopRatings`, HTTP edge `API.md` §11):** the read-only top-N ladder. **Eligibility:** a player appears only with **≥1 scored `done` match** — the query INNER-JOINs `match_players`/`matches`, so anyone with zero finished games (or only `open`/`drawing`/`judging`/`abandoned` rows) falls out naturally; no 0-games filter, no migration, no index. `resolution = 'aborted'` rows are excluded by the same join for the same reason `abandoned` is: no verdict, no Elo. **Order:** `rating desc`, then **`users.id asc`** as the stable tie-break (every account starts at 1200, so a fresh ladder would otherwise be nondeterministic). **`rank` = 1-based row number** over that ordering (an absolute position — see why `API.md` §11 can't keyset-paginate it). **Counts:** `gamesPlayed` = every `done` match (forfeits included, full-K; `abandoned` and `aborted` excluded — no Elo, no result); `wins`/`losses` derive from `winner_player_id` (`null` = tie, counted as neither, so `ties = gamesPlayed − wins − losses` — which holds only because aborts, the other null-winner rows, are filtered out). **Privacy:** `login` is **never** selected (it may be an email) — only the nullable, user-chosen `display_name`, the same rule as the §4.2 / §7 roster.

Single-player practice (§10) is not a match and never touches any of this — no Elo delta, no `rating_before`/`rating_after` snapshot, no `ListTopRatings` row. It exists to make the product playable alone, not to move a ladder other people are also on.

## 9. Live realtime — same lifecycle, now shipped

Live realtime **shipped** (`feat/ws-realtime`, 2026-07-12, `ROADMAP.md` Phase 3 back-half) as a **delivery upgrade, not a second backend** — it pushes over WS exactly the transitions §3/§4 already define; it does not change the lifecycle, and it applies to every async match (there is no separate `matches.mode = 'live'` — the WS route is available on any match id the caller is a player in).

- **What changes:** an in-process WS hub (`internal/ws`, `coder/websocket`) pushes match-room state — `match_state` (roster/deadline), `opponent_submitted`, `judging`, `result` (judged **or** forfeit), `abandoned`, and coarse presence (`opponent_connected`/`opponent_disconnected`) — the instant `internal/game` commits each transition, so both players experience them in real time instead of only on their next poll. The full wire protocol is owned by `docs/API.md` §9.
- **What does NOT change:** the states, the transitions, the prompt-pinning, the trust boundary (authoritative server render), the A/B→player mapping, and ratings are **identical**. **Postgres remains the source of truth**; the hub only *pushes* committed transitions — it never mediates a mutation, and the REST poll loop remains both the fallback transport and the source of truth if the socket is absent or drops.
- **The visibility rule (§4.2) still holds over the wire:** `match_state`/`result` frames are rebuilt **per recipient** through the same viewer-scoped read the REST handlers use (never a marshal-once broadcast), so a mid-round frame sent to player A carries A's own `drawingId` and never B's — the hub broadcasts *that* the opponent submitted, never *what* they drew, until `done`.

Because the lifecycle was delivery-agnostic, shipping live was wiring a transport over the already-proven loop — not rebuilding the game.

## 10. Practice — single-player scoring, not a match

**Why it exists.** The duel (§3/§4) needs two people **at the same moment**, and without a player base the first visitor to `/play` waited through the `open`-match TTL (§4.1, ~10 min) and got an `abandoned` match for their trouble — the product was unplayable by the person most likely to try it. Practice (`/practice`) scores **one drawing, alone**, reusing the same prompts, the same square canvas, and the same real judge infrastructure the duel already has, rather than building anything new underneath. It shipped as a Phase 4 addition, after the duel's own exit criteria were already met (`docs/ROADMAP.md`).

**Deliberately NOT a `matches` row.** `decideExpiry` (`server/internal/game/deadline.go`) partitions a locked match's roster into submitted/missing players and has exactly three outcomes: nobody submitted (`outcomeAbandoned`), exactly one of two did (`outcomeForfeit`), or its fallback, `outcomeJudging` — reached today only by "both submitted" but also, structurally, by any other roster shape, including a single-seat one. A one-player round would fall into that fallback, flip to `judging`, and wedge there forever: `runJudging` demands exactly two submissions to render and score. The duel's whole lifecycle — matchmaking, a shared deadline, forfeit, abandonment, the stuck-judging watchdog (§4.1) — exists **because** two people wait on each other; a solo player waits on nobody, so none of it applies, and reusing `matches` would mean delicate surgery on the most sensitive state machine in this codebase to support a mode that needs none of it. Practice is instead a flat table, `practice_runs` (`server/migrations/00006_practice_runs.sql`): `id, user_id, prompt_id, score, feedback, created_at` — **no `status` column, no sweeper, no deadline.**

**Scored by a separate seam, not the `Judge` contract.** `docs/JUDGE.md` §2's `Judge` is frozen and inherently comparative (`prompt, pngA, pngB → scoreA, scoreB, winner, reason`) — the agreement with the external ML collaborator. Practice asks a different question, "how well does this ONE drawing depict the prompt?", so it is served by `judge.Critic` instead — ours, explicitly not something the collaborator implements or is asked to. Full seam, including the `JUDGE_MODE` selection table: `docs/JUDGE.md` §8.2.

**Flow — synchronous, unlike a duel:**
```
GET a prompt (never redacted — no opponent to be fair to)
  → draw, on the SAME 1080×1080 canvas as a duel (§2)
  → POST the document
      budget check → authoritative render (off-client, §6) → critique
      → score + feedback, in the SAME response
```
Unlike `judging → done` (§4.1), there is no out-of-band step and nothing to poll: the one player is already waiting, so the request that submits the drawing is the request that returns the verdict. The render-plus-critique is bounded by its own budget (`practice.RunBudget`, 25s) so a slow critic cannot outlive the HTTP server's 30s write timeout and cut the response off mid-write — an overrun there would not be an error a player could read, so a clean `500` at 25s beats a dead socket at 30s.

**Spends the same daily AI-call budget as a duel — §4.3.** A practice run is one judge call against the same provider quota a duel spends, counted by the same rule over the same ledger; a ceiling that only ever counted duels would be no ceiling at all the moment a single request could spend it with no opponent required. Practice does have its **own** per-player allowance (kind `practice`, 20/day) rather than sharing the duel's — the per-provider global ceiling is the half the two modes genuinely share.

**No Elo, no leaderboard effect (§8).** Ratings exist to make a *duel* result move a ladder other people are also on; applying them to a run nobody else played would let a player farm rating alone. `practice_runs` carries no rating columns and is not read by `ListTopRatings`.

**The document is validated and rendered exactly like a submission, but never stored.** Practice runs the identical write-edge validator a duel submission does — including the 1080×1080 canvas check (§2) — and the identical authoritative server-side render (§6). Unlike a duel submission (§7), the vector document itself is **not** persisted afterward: no `drawings` row is created, and nothing survives the request except the verdict in `practice_runs`. The client's own advisory thumbnail is therefore the only picture of a practice run that outlives its response.

Full HTTP contract (routes, request/response shapes, every error code): `docs/API.md` §12. The `Critic` seam and the `JUDGE_MODE` selection table: `docs/JUDGE.md` §8.2. The decisions and their reasoning: `docs/DECISIONS.md` 2026-09-20.
