# Game — match lifecycle & product rules

> **The north star spec.** The AI-judged drawing duel (`/play`): two players draw the *same* prompt, the server renders authoritative rasters, the judge scores them, a winner (or tie) is recorded. This doc owns the **match lifecycle/state machine, the canonical game canvas size, prompt pinning, the ratings sketch, and tie handling**. It defers the judge contract wholesale to `docs/JUDGE.md` and the document/storage schema to `docs/DOCUMENT-FORMAT.md`.
>
> **Status:** Phase 0 (draft). The async duel is **v1**; live realtime (§9) has now shipped as a delivery upgrade, without forking the lifecycle. Companion: `docs/DECISIONS.md` (the "why"), `docs/ARCHITECTURE.md` (§7 data model, §8 async-first), `docs/JUDGE.md` (scoring contract), `docs/API.md` (routes, error shape, DoS caps). When this disagrees with those for what it owns, this doc wins; for what it defers, they win.

## 1. Scope & ownership

This doc **owns**:
- the match **state machine** (`open | drawing | judging | done | abandoned`) and every transition (§3, §4);
- the **canonical game canvas** = square **1080×1080** (§2);
- how a **prompt** is pinned per match (§5);
- the **trust boundary** for game submissions (§6);
- the **ratings sketch** (Elo-style) and **tie** handling (§8).

This doc **defers**:
- the **judge contract** — `Result` shape, positional `winner` type, tie rule, raster size (1024×1024), background — entirely to **`docs/JUDGE.md`**. We only consume it.
- the **`drawings` table** to **`docs/DOCUMENT-FORMAT.md` §7**; the **`users` / `prompts` / `matches` / `match_players`** columns to **`docs/ARCHITECTURE.md` §7** (this doc tightens their *semantics*, never re-declares the column names).
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
| `done` | Judge returned, **or** the round deadline passed with exactly one submitter (forfeit); result (scores, winner-or-tie, reason, and how it was decided) recorded; ratings applied. Terminal. |
| `abandoned` | Match ended without a result (timeout / a player never submitted / cancelled). Terminal. |

Only `done` and `abandoned` are terminal. `winner_player_id` is meaningful only in `done` (and may be `null` there — a tie, §8). A `done` match also carries `resolution` (`judged` | `forfeit`) — how it was decided (§4.1).

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
| `drawing` → `abandoned` | The **round deadline passes** with **zero** submitters. | Flip `status = 'abandoned'`; no scores, no rating change — nobody drew. |
| `open` → `abandoned` | An **open match sits unjoined past a TTL** (~10 min). | A background reaper sweeps stale `open` matches to `abandoned` so a ghost match can't later pair a fresh joiner against a creator who's long gone. (**Explicit cancel** is still just the optional, unimplemented `API.md` §8 `POST …/abandon` — not a shipped trigger.) |

Notes:
- A **submit into a non-`drawing` match** (e.g. already `judging`/`done`/`abandoned`) is an **illegal transition** → `409 conflict` (`docs/API.md`). The state machine, not the client, gates this.
- The `drawing → judging` flip is **all-or-nothing on the roster**: one player submitting does not advance the match; it only stamps their slot. The match advances when the *last* slot is filled.
- **Idempotent submit:** re-submitting an already-stamped slot is rejected (`409`), so a double-tap can't overwrite a submission or re-trigger judging.
- **Late submit (deadline passed):** a submit landing at or after `drawing_deadline` is rejected — `409 conflict`, message `"round expired"` — and is **not** stamped, even though the match may still read `status: drawing` at that instant (the check runs on the same Postgres clock that stamped the deadline, so it can't be raced by host-clock skew). The round resolves to forfeit/abandoned as part of rejecting the late submit, if the background sweeper hasn't already gotten to it first.

### 4.2 Visibility rule

**During a round each player sees ONLY their own canvas.** No peeking at the opponent's in-progress (or finished) drawing while the match is live. **Both canvases are revealed together on the result screen** once `status = done` (`DECISIONS.md` "Game screen visibility"). The opponent's `drawing_id` / rendered raster is not exposed by any read endpoint until the match is `done`.

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

> **Scope:** a deliberately small Elo sketch so a result *moves the needle*. **Full ratings are Phase 4** (`ROADMAP.md`) — leaderboards, decay, provisional/placement handling, anti-abuse are out of scope here. This pins just enough to apply a rating delta on every `done` match.

- **Storage:** `users.rating int not null default 1200` (the live rating); `match_players.rating_before` / `rating_after` snapshot each player's rating around the match (audit + display). All three owned by `ARCHITECTURE.md` §7.
- **Model (standard Elo):** expected score for player P against opponent O
  `E_P = 1 / (1 + 10^((rating_O − rating_P) / 400))`.
  Actual score `S_P`: **win = 1, loss = 0, tie = 0.5** (shared/half points — `DECISIONS.md` "Ties are allowed").
  New rating: `rating_after = round(rating_before + K · (S_P − E_P))`.
- **K-factor:** **K = 32** for v1 (a single flat K — simple, responsive; tiered/provisional K is Phase 4).
- **Tie:** both players take `S = 0.5`; the deltas are equal-and-opposite only when ratings were equal, otherwise the lower-rated player gains and the higher-rated loses a little, as Elo intends. `matches.winner_player_id = null`.
- **Forfeit:** if the round deadline passes with exactly one submitter (§4.1), that player's actual score is fixed at `S = 1` fed into the *same* `computeElo` — full `K = 32`, exactly like a decisive judged win, no discount. The judge never runs (only one image exists), so `match_players.score` (the judge similarity) stays `null` for both players; the S-value lives only in the Elo math, not in that column. `matches.resolution = 'forfeit'` distinguishes it from `'judged'`.
- **When applied:** exactly once, atomically, on the `judging → done` transition (a judged result) **or the `drawing → done` forfeit transition** (§4.1) — after the result is recorded. `rating_before` is captured before the update; `rating_after` after. An `abandoned` match applies **no** rating change.
- **Atomic ladder write:** the delta reaches `users.rating` via `rating = rating + delta` (`ApplyRatingDelta`, `RETURNING` the true post-value), **never** an absolute `SET`, so two matches seating the same player and resolving concurrently both land — the match row lock serializes per *match*, not per *user* (`docs/NOTES.md`, `DECISIONS.md` 2026-07-13). The delta *magnitude* is sized from a pre-match rating snapshot (a rating period — standard Elo), while `rating_before`/`rating_after` are derived from that atomic write's `RETURNING` so the snapshot always agrees with the ladder.
- **Outcome from the judge, not the score gap:** win/loss/tie is taken from the judge's `winner` field (mapped per §7.1), not by comparing `scoreA`/`scoreB` ourselves — the judge owns the verdict, including whether a near-equal pair is a tie. (A forfeit has no judge outcome to take — the winner is simply the submitter.)

## 9. Live realtime — same lifecycle, now shipped

Live realtime **shipped** (`feat/ws-realtime`, 2026-07-12, `ROADMAP.md` Phase 3 back-half) as a **delivery upgrade, not a second backend** — it pushes over WS exactly the transitions §3/§4 already define; it does not change the lifecycle, and it applies to every async match (there is no separate `matches.mode = 'live'` — the WS route is available on any match id the caller is a player in).

- **What changes:** an in-process WS hub (`internal/ws`, `coder/websocket`) pushes match-room state — `match_state` (roster/deadline), `opponent_submitted`, `judging`, `result` (judged **or** forfeit), `abandoned`, and coarse presence (`opponent_connected`/`opponent_disconnected`) — the instant `internal/game` commits each transition, so both players experience them in real time instead of only on their next poll. The full wire protocol is owned by `docs/API.md` §9.
- **What does NOT change:** the states, the transitions, the prompt-pinning, the trust boundary (authoritative server render), the A/B→player mapping, and ratings are **identical**. **Postgres remains the source of truth**; the hub only *pushes* committed transitions — it never mediates a mutation, and the REST poll loop remains both the fallback transport and the source of truth if the socket is absent or drops.
- **The visibility rule (§4.2) still holds over the wire:** `match_state`/`result` frames are rebuilt **per recipient** through the same viewer-scoped read the REST handlers use (never a marshal-once broadcast), so a mid-round frame sent to player A carries A's own `drawingId` and never B's — the hub broadcasts *that* the opponent submitted, never *what* they drew, until `done`.

Because the lifecycle was delivery-agnostic, shipping live was wiring a transport over the already-proven loop — not rebuilding the game.
