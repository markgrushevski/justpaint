# Judge contract

> **The agreement with the external ML collaborator.** The judge is built by a collaborator as his own portfolio piece — **we never build the ML.** We own only this contract and a fake implementation. This document is the single source of truth for the judge's exact shape: the request/response, the **positional `winner` semantics**, tie rule, the judged-raster spec (size + background), and the transport. If code or another doc disagrees with this file, **this file wins** for everything judge-related.
>
> **Ownership note.** `docs/ARCHITECTURE.md` §5, `docs/DOCUMENT-FORMAT.md` §10, and `docs/GAME.md` all *defer* to this doc for the `winner` representation, tie semantics, raster size, and background. This doc, in turn, defers the trust boundary / render pipeline to `DOCUMENT-FORMAT.md` §10 and the match lifecycle / A·B→player resolution to `GAME.md`.
>
> **Status:** v1 contract, Phase 0 (draft). Greenfield. The collaborator can integrate against this alone — he never reads our other docs, never parses our document schema, never runs `getStroke`.

## 1. What the judge is (and what it is NOT)

The judge is a **pure function over two images and a prompt**:

```
judge(prompt, pngA, pngB) → { scoreA, scoreB, winner, reason }
```

- **It scores rasters, not drawings.** It receives two **pre-rendered PNGs** that *we* produced authoritatively server-side from each player's vector document. It never sees the vector document, never imports `packages/document`, never runs `perfect-freehand`/`getStroke`. This deletes every cross-language determinism and version-coupling problem from the ML boundary (`DOCUMENT-FORMAT.md` §10 trust boundary; `ARCHITECTURE.md` §5).
- **It has no notion of users, matches, or ratings.** It speaks only in **positional `A`/`B`** over the two images it was handed. The `game` module maps `A`/`B`/`tie` to concrete player ids at submit time (§4, `GAME.md` §7.1).
- **It is stateless and side-effect-free.** Same `(prompt, pngA, pngB)` ⇒ same result (the `FakeJudge` guarantees this; the real ML should aim for it — §9). It stores nothing, owns no database, and is the only external service in the system (`ARCHITECTURE.md` §1).

**Why this shape:** the duel must be playable with **zero ML dependency** (a `FakeJudge`, §8) and the real judge must be swappable behind one interface (`HTTPJudge`, §7) with **no change to the schema or the game loop**. Never block on the ML (`DECISIONS.md` "The ML judge is external").

## 2. The contract (canonical)

The judge receives a prompt and two images, and returns two scores, a positional winner, and a human-readable reason.

```ts
interface JudgeRequest {
  /** The match prompt — the SAME text both players drew (GAME.md §5 pins one prompt per match). */
  prompt: string;
  /** Pre-rendered authoritative PNGs, square 1024×1024, opaque background (§5). Positional. */
  imageA: Image;
  imageB: Image;
}

interface JudgeResult {
  /** Similarity of image A to the prompt. Range [0, 1]; higher = better match. */
  scoreA: number;
  /** Similarity of image B to the prompt. Range [0, 1]; higher = better match. */
  scoreB: number;
  /** POSITIONAL verdict over the two images — NOT a player id. "tie" is allowed (§3). */
  winner: "A" | "B" | "tie";
  /** Short human-readable rationale, shown on the result screen. ≤500 chars, plain text. */
  reason: string;
}
```

**Field meanings (pinned):**

| Field | Type | Meaning / constraints |
|---|---|---|
| `prompt` | string | The shared prompt text. Non-empty. The judge may use it freely; the `FakeJudge` ignores it (§8). |
| `imageA` / `imageB` | image (bytes or URL, §6) | The two PNGs to compare. **Positional** — `A`/`B` are just "first image / second image", assigned by `game`. |
| `scoreA` / `scoreB` | number ∈ `[0, 1]` | Per-image similarity-to-prompt. **Higher is better.** Finite (no `NaN`/`±Inf`). The two scores are independent; they need **not** sum to 1. |
| `winner` | `"A"` \| `"B"` \| `"tie"` | The decisive positional label. See §3 for the tie rule and its relationship to the scores. |
| `reason` | string | Why this verdict — surfaced to players as the judge's explanation. Plain text, ≤500 chars, no markup. Must not assume any player identity (it may say "the left/first drawing", never a username). |

**Score semantics.** `scoreA`/`scoreB` are absolute similarity-to-prompt readings in `[0, 1]`, **not** relative shares. Two great drawings can both score `~0.9`; two poor ones both `~0.1`. `winner` is the judge's decisive call and is **authoritative** — the game uses `winner` for the result, and the scores for display/ratings. (If the real ML's natural output is unbounded, it MUST normalize into `[0, 1]` before returning — §9.)

## 3. The `winner` field — positional, with ties allowed

- **Positional, not a player id.** `winner ∈ {"A", "B", "tie"}` refers to the *images*, in the order they were sent. The judge has no idea who drew what. The `game` module records which player's drawing was rendered as `imageA` vs `imageB` and resolves the label to `matches.winner_player_id` (§4, `GAME.md` §7.1).
- **Ties ARE allowed.** `winner: "tie"` is a first-class outcome — the duel does **not** force a tiebreak (`DECISIONS.md` "Ties are allowed"). On a tie, `matches.winner_player_id` is **null** (nullable by design — `ARCHITECTURE.md` §7) and ratings award **shared/half points** (`GAME.md` §8 owns the tie-rating rule).
- **`winner` is authoritative over the scores.** The game keys the result off `winner`, not off comparing `scoreA`/`scoreB`. The judge decides where a near-equal pair lands (decisive vs. tie) — it owns the tie threshold internally; we do not impose one. (Consumers should therefore **not** re-derive the winner from the scores; a judge may legitimately call `0.71` vs `0.70` a `"tie"`.)
- **Internal consistency expected, not enforced.** A well-behaved judge returns `"A"` when `scoreA > scoreB`, `"B"` when `scoreB > scoreA`, and `"tie"` when they are within its own tolerance. The game does not reject a result that violates this — `winner` is taken as-is — but the collaborator should keep them consistent so the reason and scores read sensibly to players.

## 4. A·B → player mapping (owned by `game`, not the judge)

The judge is positional; the resolution lives entirely in the `game` module (`ARCHITECTURE.md` §5, `GAME.md` §7.1):

1. At submit, both players' vector documents are rendered to authoritative PNGs (`DOCUMENT-FORMAT.md` §10).
2. `game` assigns one player's PNG to `imageA` and the other's to `imageB`, **remembering the mapping** for this match.
3. It calls `Judge.Score` and receives a positional `winner`.
4. It maps back: `"A"` → player-A id, `"B"` → player-B id, `"tie"` → **null**. Writes `matches.winner_player_id`, `match_players.score` (from `scoreA`/`scoreB`), and `matches.judge_reason` (from `reason`). See `ARCHITECTURE.md` §7 for those columns; `GAME.md` §4/§7.1 for the lifecycle.

The judge never learns the mapping and must never return a player id. Keeping it positional is what lets the same judge serve any pairing (and, later, teams) without change.

## 5. The judged raster (we render it; the judge consumes it)

The images are produced by **us**, authoritatively, off the player's machine — the judge only consumes them.

- **`JUDGE_FRAME = 1024 × 1024`, square.** Both images are exactly this size (`DECISIONS.md` "Square canvas + square judge frame"). Chosen square to pair with the square `GAME_CANVAS = 1080 × 1080` (`GAME.md` §2) so there is **no letterbox** — letterbox bars carry no drawing yet count as judged pixels and would skew similarity (`DOCUMENT-FORMAT.md` §2 scope note).
- **Format: PNG**, 8-bit RGBA (the alpha is fully opaque after the background fill below).
- **Opaque background, recommended white (`#ffffff`).** The render forces an opaque background for the judged raster via `RenderOptions.background`, which **replaces** the document's own `background` (`DOCUMENT-FORMAT.md` §10). Determinism + a known backdrop matters: a transparent or document-chosen background would make ink-coverage and contrast readings non-comparable across the two images. Both images use the **same** forced background.
- **Fit: `contain`, centered.** The 1080² game canvas is scaled-to-fit and centered into the 1024² frame using the pinned contain transform (`DOCUMENT-FORMAT.md` §10). Aspect is preserved, never stretched. Because both are square the scale is uniform and the margin is ~0; the forced background fills any residual margin.
- **Authoritative, never client-supplied.** The PNG is rendered server-side from the submitted **vector document** by the Node render worker that shares `packages/document` (`DOCUMENT-FORMAT.md` §10, `ARCHITECTURE.md` §8/§9). A client thumbnail may exist for instant UI but is **advisory only** — a cheater could doctor it; it is never sent to the judge (trust boundary, `DOCUMENT-FORMAT.md` §10, `GAME.md` §6).

The collaborator can assume: **two same-size square PNGs, opaque background, drawing centered, no transparency to reason about.** He does not need to know our canvas size, fit math, or document format — only that he receives two comparable 1024² PNGs.

## 6. Transport (HTTP)

The `HTTPJudge` (§7) calls the collaborator's service over HTTP/JSON. This section is the wire agreement.

**Request.** `POST {judgeBaseURL}/v1/score`, `Content-Type: application/json`:

```json
{
  "prompt": "a fox riding a bicycle",
  "imageA": "<image>",
  "imageB": "<image>"
}
```

**Image delivery — two supported modes (the collaborator picks one; we configure to match):**

- **Inline base64 (default for v1):** `imageA`/`imageB` are base64-encoded PNG bytes as JSON strings (optionally a `data:image/png;base64,` prefix). Simplest to integrate; at 1024² a PNG payload is small. This is the v1 default because it needs no shared object storage between us and the collaborator.
- **URL (when object storage lands):** `imageA`/`imageB` are signed, short-TTL `https://` URLs to the PNGs in our object storage (`ARCHITECTURE.md` §7/§9). The judge fetches them. Preferred once object storage exists, to keep request bodies tiny. URLs must be treated as opaque and fetched read-only.

A request carries **both images the same way** (both inline or both URL), never mixed. Which mode is active is a deployment config on our side (`JUDGE_IMAGE_MODE`, §7); the request shape is otherwise identical.

**Response.** `200 OK`, `Content-Type: application/json`, body = the `JudgeResult` of §2:

```json
{ "scoreA": 0.82, "scoreB": 0.61, "winner": "A", "reason": "A clearly shows a fox on a bike; B reads as an abstract blob." }
```

**Headers.**
- `X-Judge-Contract-Version: 1` on requests (§10). The judge SHOULD echo it on responses.
- `Idempotency-Key: <match-submit id>` on requests — the same key for retries of the same scoring (§7). The judge MAY use it to dedupe; safe to ignore since scoring is pure.

**Errors.** On failure the judge returns a non-2xx with a JSON body that mirrors our envelope *shape* (`{ "error": { "code", "message" } }`, `API.md` §3):

```json
{ "error": { "code": "bad_request", "message": "imageB failed to decode as PNG" } }
```

> **These `code` values are the judge service's own** — `bad_request` (malformed body / undecodable image / not 1024²), `unsupported_media` (image not PNG), `internal` (model failure). They are **independent of `API.md`'s closed v1 code set (§3)**; the judge is an external service, not part of justpaint's API surface, so it is not bound by that set. We only require the `{ error: { code, message } }` shape so our `HTTPJudge` can log a structured failure. A 5xx or timeout is treated by us as a transient failure (§7) and **never** as a verdict.

## 7. Our side — the Go `Judge` interface and `HTTPJudge`

`internal/judge` mirrors this contract (canonical types live here in `JUDGE.md`; `ARCHITECTURE.md` §5 shows the same interface):

```go
// internal/judge — mirrors docs/JUDGE.md (this doc owns the canonical shape).
type Judge interface {
    Score(ctx context.Context, req Request) (Result, error)
}

type Request struct {
    Prompt string
    ImageA []byte // authoritative pre-rendered PNG, 1024×1024, opaque bg (§5)
    ImageB []byte
}

type Result struct {
    ScoreA float64 // [0,1]
    ScoreB float64 // [0,1]
    Winner string  // positional: "A" | "B" | "tie" (§3)
    Reason string
}
```

- **`Request` carries bytes** in Go regardless of wire mode: the `HTTPJudge` base64-encodes them (inline mode) or uploads + sends URLs (URL mode). The `game` module always hands the judge bytes and stays ignorant of transport.
- **`game` depends on the `Judge` interface, not the impl** (`ARCHITECTURE.md` §4/§5) — it does not know whether the judge is fake, in-process, or HTTP.

**`HTTPJudge` behavior (pin these):**
- **Timeout.** A per-call deadline via `ctx` — default **10s** (ML inference may be slow; tune by config). On deadline exceeded → transient error.
- **Retries.** Up to **2 retries** (3 attempts total) on connection errors, timeouts, and `5xx`, with backoff. **No retry on `4xx`** (a 4xx means our request is wrong — fix it, don't hammer). Scoring is pure, so retries are safe; the `Idempotency-Key` lets the judge dedupe if it wants.
- **Failure is not a verdict.** If all attempts fail, the call returns an `error`; `game` does **not** invent a winner. The match stays in `judging` and the submit step surfaces a retryable failure (`GAME.md` §3/§4 owns the state handling; `API.md` §8.3 owns the HTTP error to the client). The fake judge (§8) means dev/CI never hit this path.
- **Strict response validation.** Reject a `200` whose body violates §2 (scores outside `[0,1]`, non-finite, `winner` not in the enum, `reason` too long) — treat as a contract violation, not a verdict.
- **Configured by env**, swapped with the fake at composition time (`main` wiring, `ARCHITECTURE.md` §4): `JUDGE_MODE=fake|http`, `JUDGE_BASE_URL`, `JUDGE_TIMEOUT`, `JUDGE_IMAGE_MODE=inline|url`.

## 8. `FakeJudge` (default in dev/CI — zero ML dependency)

The whole loop (create → draw → submit → judge → result → ratings) must ship and demo before the ML exists. `FakeJudge` is the **default** implementation in dev and CI (`ARCHITECTURE.md` §5, `DECISIONS.md`).

**Requirements:**
- **Deterministic.** Same `(prompt, imageA, imageB)` ⇒ identical `Result`, every run. No randomness without a fixed seed. This keeps tests stable and matches reproducible.
- **Heuristic, prompt-independent is fine.** It need not understand the prompt. A good v1 heuristic: decode each PNG and compute **ink coverage** = fraction of non-background pixels (against the known opaque background, §5); map coverage → `score ∈ [0,1]`; the higher score wins; declare `"tie"` when `|scoreA − scoreB|` is within a small epsilon. A pure seeded hash of the image bytes is an acceptable simpler variant.
- **Honors the contract exactly.** Returns scores in `[0,1]`, a valid positional `winner` (including `"tie"`), and a plain-text `reason` (e.g. `"A covers 38% of the canvas vs B's 22% — A wins on ink coverage (fake judge)."`). It exercises every consumer path — including ties — so swapping in `HTTPJudge` is purely a config change.

`FakeJudge` runs **in-process** (no HTTP), needs no network, and is what the Phase 3 async-duel exit criteria are demonstrated against (`ROADMAP.md` Phase 3).

## 9. Expectations on the real ML judge (for the collaborator)

The contract is the hard boundary; these are the soft requirements that keep matches fair:

- **Bound the scores.** Return `scoreA`/`scoreB` in `[0, 1]`, finite. If the model emits unbounded logits/distances, normalize before returning — do not leak raw model scale.
- **Determinism is strongly preferred.** Same inputs ⇒ same output. Non-determinism (e.g. sampling) makes a replay/audit score differ from the live one; if unavoidable, fix a seed. We render byte-identical PNGs on our side specifically so the only remaining variance is the model's (`DOCUMENT-FORMAT.md` §1 design goal).
- **Positional fairness.** Don't bias toward `imageA` or `imageB` by position. We may, for audit, send the same pair swapped and expect the verdict to swap accordingly (`A`↔`B`, `tie` stable).
- **Latency.** Aim well under the 10s timeout (§7); the async duel tolerates seconds, not minutes.
- **The `reason` is player-facing.** Keep it short, plain, and free of player identity (positional or generic language only). It is shown verbatim on the result screen.
- **Stateless.** No persistence, no per-user memory; treat each request independently.

What the collaborator can rely on from us: two comparable square 1024² PNGs, opaque background, drawing centered, always sent the same way; a stable request/response shape; retries that are safe because scoring is pure.

## 10. Versioning

- **Contract version is an integer, signalled by `X-Judge-Contract-Version` (v1 = this doc).** Independent of the document-format `version` (`DOCUMENT-FORMAT.md` §9) and of any API route version.
- **Additive changes do not bump it:** new optional response fields (e.g. a future `confidence`, per-region scores), new optional request hints. Both sides ignore unknown fields — the judge MUST tolerate extra request fields, and our `HTTPJudge` MUST tolerate extra response fields (forward-compat, mirroring `DOCUMENT-FORMAT.md` §7 "allow unknown fields").
- **Breaking changes bump it:** changing the score range/meaning, the `winner` enum, the raster size/background, the image-delivery contract, or making `reason` structured. On a bump we coordinate with the collaborator and run both versions behind config until cutover.
- **The judged-raster spec (size 1024², opaque background, contain-fit) is part of this contract.** A change to it is a contract bump and forces re-rendering of any cached judged PNGs (consistent with `DOCUMENT-FORMAT.md` §9 render-contract changes).

## 11. Open items (owned elsewhere, consumed here)

- **Match lifecycle, submit step, ratings, tie-point rule, A·B→player resolution** → `GAME.md`. This doc only defines the function the submit step calls.
- **Render pipeline, fit math, per-layer isolation, trust boundary, `RenderOptions`** → `DOCUMENT-FORMAT.md` §10. This doc only states the *spec* of the PNG it receives (size/background/format).
- **HTTP error envelope, status codes, object-storage URLs, auth** → `API.md` §3 / `ARCHITECTURE.md` §7/§9.
- **The ML itself** → the collaborator. We never build it; we only define this contract and ship the `FakeJudge`.
