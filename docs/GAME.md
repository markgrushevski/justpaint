# Game — match lifecycle & product rules

> **The north star spec.** The AI-judged drawing duel (`/play`): two players draw the *same* prompt, the server renders authoritative rasters, the judge scores them, a winner (or tie) is recorded. This doc owns the **match lifecycle/state machine, the canonical game canvas size, prompt pinning, the ratings sketch, and tie handling**. It defers the judge contract wholesale to `docs/JUDGE.md` and the document/storage schema to `docs/DOCUMENT-FORMAT.md`.
>
> **Status:** Phase 0 (draft). The async duel is **v1**; live realtime layers on later (§9) without forking the lifecycle. Companion: `docs/DECISIONS.md` (the "why"), `docs/ARCHITECTURE.md` (§7 data model, §8 async-first), `docs/JUDGE.md` (scoring contract), `docs/API.md` (routes, error shape, DoS caps). When this disagrees with those for what it owns, this doc wins; for what it defers, they win.

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
| `done` | Judge returned; result (scores, winner-or-tie, reason) recorded; ratings applied. Terminal. |
| `abandoned` | Match ended without a result (timeout / a player never submitted / cancelled). Terminal. |

Only `done` and `abandoned` are terminal. `winner_player_id` is meaningful only in `done` (and may be `null` there — a tie, §8).

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

| From → To | Trigger | Server actions |
|---|---|---|
| *(none)* → `open` | A player **creates** a match. | Insert `matches` row; **pin exactly one prompt** (§5) → `matches.prompt_id`; set `mode = 'async'`, `status = 'open'`; insert the creator's `match_players` row. |
| `open` → `drawing` | The **roster fills** (second player joins a 1v1). | Insert the second `match_players` row; flip `status = 'drawing'`. The prompt is now revealed to both. |
| `drawing` → `judging` | The **last** outstanding player **submits**. | Persist each submission as a `drawings` row (`match_id` set), stamp `match_players.drawing_id` + `submitted_at`. When all roster slots have a submission, flip `status = 'judging'` and kick off rendering. |
| `judging` → `done` | The **judge returns**. | Render authoritative PNGs (§6) → call the judge (`docs/JUDGE.md`) → write `match_players.score`, map positional `winner` → `matches.winner_player_id` (null on tie, §7.1), store `matches.judge_reason`; apply ratings (§8); flip `status = 'done'`. |
| `open`/`drawing` → `abandoned` | **Timeout** or **explicit cancel** before both submit. | Flip `status = 'abandoned'`; no scores, no rating change. A half-drawn `drawings` row may persist (advisory, unjudged). |

Notes:
- A **submit into a non-`drawing` match** (e.g. already `judging`/`done`/`abandoned`) is an **illegal transition** → `409 conflict` (`docs/API.md`). The state machine, not the client, gates this.
- The `drawing → judging` flip is **all-or-nothing on the roster**: one player submitting does not advance the match; it only stamps their slot. The match advances when the *last* slot is filled.
- **Idempotent submit:** re-submitting an already-stamped slot is rejected (`409`), so a double-tap can't overwrite a submission or re-trigger judging.

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
- **When applied:** exactly once, atomically, on the `judging → done` transition — *after* the judge result is recorded. `rating_before` is captured before the update; `rating_after` after. An `abandoned` match applies **no** rating change.
- **Outcome from the judge, not the score gap:** win/loss/tie is taken from the judge's `winner` field (mapped per §7.1), not by comparing `scoreA`/`scoreB` ourselves — the judge owns the verdict, including whether a near-equal pair is a tie.

## 9. Live mode — same lifecycle, later

Live realtime is **not v1** (`ARCHITECTURE.md` §8; `ROADMAP.md` Phase 3 back-half). It is a **delivery upgrade, not a second backend** — async and live share the **one** lifecycle in §3/§4.

- **What changes:** `matches.mode = 'live'`; a WS hub (`internal/ws`, coder/websocket — `docs/API.md` sketches the protocol, marked not-v1) pushes match-room events (**opponent joined, opponent submitted, judging, result**) so both players experience the transitions in real time instead of polling.
- **What does NOT change:** the states, the transitions, the prompt-pinning, the trust boundary (authoritative server render), the A/B→player mapping, and ratings are **identical**. **Postgres remains the source of truth**; the hub only *pushes* transitions the lifecycle already defines — it never owns them.
- **The visibility rule (§4.2) still holds in live:** players see only their own canvas during the round; the hub may broadcast *that* the opponent submitted, never *what* they drew, until `done`.

Because the lifecycle is delivery-agnostic, shipping live is wiring a transport over an already-proven loop — not rebuilding the game.
