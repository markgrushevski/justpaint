# Architecture

> **System topology & boundaries.** How the pieces fit, which way dependencies point, and where the seams are. Companion to `docs/DECISIONS.md` (the "why" of each call) and `docs/DOCUMENT-FORMAT.md` (the keystone contract). This doc maps the *structure*; it does not relitigate the decisions that produced it.
>
> **Status:** Phase 0 (draft). The target layout below **does not exist yet** — the repo is mid-migration from the old `client/` (Vue raster) + `server/` (NestJS). This describes where we're going, plus explicit triggers for when to split further (§9).

## 1. One picture

```
┌───────────────────────────── apps/web (Vue 3 SPA) ─────────────────────────────┐
│   /draw  (free editor)            /play  (the game: async duel first)           │
│        └──────────── packages/editor (Konva + perfect-freehand) ───────────┐    │
│                              └── packages/document (schema · (de)serialize · render→PNG)
└────────────────────────────────────────┬───────────────────────────────────────┘
                                          │ HTTP/JSON (+ WS later)
                                          ▼
┌───────────────────────── server/ (Go modular monolith, one binary) ────────────┐
│   auth · drawings · game · ws-hub · judge-client          [internal/ modules]   │
│            └──────────────── platform (db · http · config · log) ───────────────│
└──────────┬──────────────────────────────┬──────────────────────┬───────────────┘
           │ pgx/sqlc                      │ object storage        │ HTTP (Judge contract)
           ▼                               ▼                       ▼
     ┌───────────┐                  ┌──────────────┐      ┌──────────────────────┐
     │ Postgres  │                  │ rendered PNGs│      │  ML judge (external,  │
     │  (one DB) │                  │ (later)      │      │  collaborator-owned)  │
     └───────────┘                  └──────────────┘      └──────────────────────┘
```

Everything left of the Judge box is ours, in one repo, shipping as **one Go binary + one static frontend + one Postgres**. The judge is the only external service.

## 2. Monorepo layout & why package boundaries (not separate repos)

```
packages/document/   # vector doc schema + (de)serialize + render→PNG   — the contract
packages/editor/     # Konva + perfect-freehand: tools, layers, history, export
apps/web/            # Vue app: /draw (free) + /play (game)
server/              # Go modular monolith: auth + drawings + game + ws hub + judge client
docs/                # specs & agreements (source of truth)
```

**Reuse = package boundaries, not repos.** The editor is consumed by *both* `/draw` and `/play`; the document schema is consumed by the editor, the Go backend, and (via rendered PNGs) the judge. We express that shared-ness as **internal packages with explicit public APIs**, not as separate npm-published repos.

Why not split into multiple repos now:
- **One source of truth, atomic changes.** A change to the keystone document format touches the schema, the editor's renderer, and the Go validator in *one commit* — no cross-repo version dance, no "which repo is canonical" drift. The format is the contract; it must move as a unit.
- **The reusability signal is already there.** The `editor` package boundary *is* the portfolio evidence of reuse (DECISIONS: "the reusability signal already comes from the `editor` package boundary"). A second repo would add ops/release overhead without adding architectural signal.
- **Solo-project economics.** Multi-repo = multi-CI, multi-version, multi-publish for one maintainer. A monorepo with clean internal seams gets the boundary discipline without the tax.

The discipline is enforced by **dependency direction** (§3), not by repo walls. If a package can't import "up", the boundary is real even in one repo.

## 3. Dependency direction (the rule that keeps boundaries honest)

Dependencies point **toward the contract**, never back toward the app:

```
apps/web ─▶ packages/editor ─▶ packages/document
                                      ▲
server/ (Go) ─────────── mirrors ─────┘   (independent reimplementation of the same spec)
```

- **`packages/document`** depends on nothing internal. It is pure: types, (de)serialize, validation, and a `render→PNG` renderer. It must not import `editor`, `web`, or anything UI/transport.
- **`packages/editor`** depends on `document` + Konva + perfect-freehand. It owns tools, layer UI, command-based undo/redo, and the `document → Konva` projection. It does **not** depend on `web` (no Vue, no router, no API client).
- **`apps/web`** depends on `editor` and the HTTP client. It owns routes (`/draw`, `/play`), Pinia stores, TanStack Query, and the game UI flow.
- **`server/` (Go)** does *not* import the TS packages. It **mirrors** the document spec (a hand-written validator for v1; a shared JSON Schema later). Both sides validate against *the spec*, not against each other's code — the spec in `docs/DOCUMENT-FORMAT.md` is the joint authority.

This is what makes `editor` genuinely reusable: because it can't reach into `web`, dropping it into a second consumer (the game vs. the free editor — or, later, a separate app) is mechanical.

## 4. The Go modular monolith (one binary, internal modules)

One Go service, one Postgres (DECISIONS: "one Go service … one Postgres"). Microservices would be a portfolio anti-pattern at this scale and would not advance the learn-Go goal. Internal structure is **modules under `internal/`**, wired together in `main`:

```
server/
  cmd/justpaint/main.go        # compose modules, start http (+ ws) server
  internal/
    auth/        # signup/login, bcrypt, golang-jwt issue/verify, middleware
    drawings/    # CRUD over the vector document (jsonb); validate on write; render trigger
    game/        # match lifecycle: create → both draw → submit → judge → result; ratings
    judge/       # Judge interface + fake impl + HTTP client to the external service (§5)
    ws/          # coder/websocket hub for the game (realtime later; async-first)
    platform/    # shared infra: pgx pool, http server/router, config, slog, errors
  migrations/    # goose
  queries/       # sqlc input → generated typed Go
```

Module rules:
- **Modules talk through narrow Go interfaces**, not by reaching into each other's internals. `game` depends on a `judge.Judge` interface and a `drawings` read port; it does not know the judge is HTTP or that drawings live in jsonb.
- **`platform` is the only shared-infra dependency.** It owns the pgx pool, router, config, and logger so modules don't each re-wire infrastructure.
- **Persistence:** pgx v5 + sqlc (typed queries) + goose (migrations). The `document` column is `jsonb`, bound as `json.RawMessage` (opaque to SQL); queryable fields are promoted to columns (§7, and `docs/DOCUMENT-FORMAT.md` §7).
- **One process, clean seams** means a module can later become its own binary by lifting it out behind its existing interface — but only when a trigger in §9 fires.

Do **not** invest in the old NestJS `server/` — it is being replaced wholesale (DECISIONS: "Don't refactor the old NestJS — replace it").

## 5. The Judge seam (interface + fake + external service)

The ML judge is **external**, built by a collaborator as his own portfolio piece (DECISIONS). We never build the ML; we own only the **contract** and a fake. **`docs/JUDGE.md` is the single owner of the contract's exact shape** — the `winner` representation, tie semantics, raster size, and background. The Go interface below mirrors it; if they ever diverge, JUDGE.md wins.

```go
// internal/judge — mirrors docs/JUDGE.md; that doc owns the canonical types.
type Judge interface {
    Score(ctx context.Context, req Request) (Result, error)
}

type Request struct {
    Prompt string
    ImageA []byte // pre-rendered PNG (authoritative raster), not the vector doc
    ImageB []byte
}
type Result struct {
    ScoreA float64
    ScoreB float64
    Winner string // positional: "A" | "B" | "tie" — type/tie-rules pinned in JUDGE.md
    Reason string
}
```

**Positional `winner` → player id (resolved by `game`, not the judge).** The judge speaks only in positional `A`/`B` over the two PNGs it was handed; it has no notion of users. The `game` module knows which player's drawing was rendered as image A vs B at submit time, so it maps `Result.Winner` to a concrete player and writes `matches.winner_player_id` (§7). **Tie handling is pinned in `docs/JUDGE.md` and `docs/GAME.md`:** `"tie"` is allowed (no forced tiebreak), `matches.winner_player_id` is null on a tie, and ratings award shared/half points (Elo S = 0.5).

Three implementations behind one interface:
- **`FakeJudge`** — deterministic/heuristic stand-in (e.g. ink coverage, seeded score). Lets the *entire* game loop (create → draw → submit → judge → result → ratings) ship and demo with **zero dependency on the ML**. This is the default in dev and CI.
- **`HTTPJudge`** — calls the collaborator's external service over HTTP against the live contract. Swapped in by config; everything else is identical.
- The collaborator integrates over HTTP against the contract; he **never parses our document format or runs `getStroke`**. He receives **pre-rendered PNGs** (§6, render path) and returns `{scoreA, scoreB, winner, reason}`.

Why this shape:
- **Never block on the ML** (DECISIONS). The fake keeps us unblocked indefinitely.
- **Clean ML boundary.** Handing over PNGs (not the vector doc) removes all cross-language determinism/version coupling from the ML side — the judge is a pure `(prompt, pngA, pngB) → result` function.
- **Trust.** The scored PNGs are rendered authoritatively server-side from the submitted document, not taken from the client (§6).

## 6. The document format as the shared contract

`packages/document` is the **keystone** (full spec: `docs/DOCUMENT-FORMAT.md`). It is the single schema shared by three consumers with three different needs:

| Consumer | Needs from the document |
|---|---|
| **editor** (`packages/editor`) | project to Konva nodes; edit via id-keyed commands; export preview PNG |
| **Go backend** (`internal/drawings`) | store as `jsonb` (opaque); validate shape/invariants on write; promote queryable fields |
| **judge** (external) | a **deterministic raster** of the drawing — receives a pre-rendered PNG, never the doc |

Load-bearing properties (all specified in `DOCUMENT-FORMAT.md`):
- **Canonical & versioned.** Mandatory integer `version`, first field, mirrored to a `doc_version` column. Renderer-agnostic — **never** Konva's `Stage.toJSON()` as storage (vendor lock; it's a dev-debug convenience only).
- **Deterministic render→PNG is a first-class capability of `packages/document`**, used in three places so they are byte-aligned by construction: editor preview (browser/Konva), the authoritative **server render worker** (Node, importing `packages/document` + Konva-node/`node-canvas` + the pinned perfect-freehand), and therefore the judge image. The render contract pins the perfect-freehand version, the brush-option subset, the freehand fill method, the contain-fit transform, and per-layer surface isolation (DOCUMENT-FORMAT §5.3/§6/§10). A Go-native second rasterizer is a trap (a renderer to keep pixel-identical) — avoided.
- **Trust boundary (game-critical):** the client submits the **vector document, never a scored PNG**. A client thumbnail may ride along for instant UI but is advisory. The judged raster is rendered off the player's machine from the document, then handed to the judge.

## 7. Data-model sketch

Promote anything queried/sorted to real columns; keep the picture in `jsonb`. The **`drawings` table is owned by `docs/DOCUMENT-FORMAT.md` §7** (the keystone storage contract) — it is *not* re-declared here to avoid drift; the sketch below references it and adds the game/identity tables. Match/rating specifics are pinned in `docs/GAME.md`.

```sql
-- drawings: SEE docs/DOCUMENT-FORMAT.md §7 (authoritative DDL).
--   Relevant columns: id, owner_id → users, match_id → matches (null for free /draw saves),
--   doc_version, width, height, document jsonb, thumbnail_url (cached preview PNG), timestamps.

users (
  id uuid pk, login citext unique, password_hash text,   -- login = email OR nickname; bcrypt; NO plaintext (old red flag)
  display_name text null, rating int not null default 1200,  -- display_name optional
  created_at, updated_at
)

prompts (
  id uuid pk, text text not null,
  active bool not null default true, created_at
)

matches (
  id uuid pk, prompt_id uuid → prompts,
  mode text not null,                      -- 'async' (v1); 'live' later
  status text not null,                    -- 'open'|'drawing'|'judging'|'done'|'abandoned'
  winner_player_id uuid null,              -- resolved from judge's positional winner (§5); null for tie/undecided per GAME.md
  judge_reason text null,
  created_at, updated_at
)

match_players (
  match_id uuid → matches, user_id uuid → users,
  drawing_id uuid null → drawings,         -- their submission
  score double precision null,             -- from the judge
  rating_before int null, rating_after int null,
  submitted_at timestamptz null,
  primary key (match_id, user_id)
)
```

Notes:
- **`match_players`** (join, two rows/match for 1v1) generalizes cleanly to teams/tournaments later without reshaping `matches`.
- **`drawings`** serves both modes: a free `/draw` save has `match_id = null`; a duel submission points at its match.
- **`matches.winner_player_id`** is the resolved player id (§5 maps the judge's positional `A`/`B`/`tie` onto it). Tie semantics (whether it can be null) are a `docs/JUDGE.md` / `docs/GAME.md` decision.
- **Rendered PNGs** (`drawings.thumbnail_url`, plus the judged raster) live in **object storage**, referenced by URL — not in Postgres (DOCUMENT-FORMAT rejects bytea/base64-PNG storage; the old `bytea`-per-layer red flag dies here).

## 8. Realtime (async-first, WS hub later)

- **v1 is the async duel:** create match → both players draw independently → submit → server renders rasters → judge → result. This needs only HTTP; **no realtime required** to ship the core loop.
- **The WS hub lives *inside* `server/`** (`internal/ws`, coder/websocket) when live mode arrives — a hub of match rooms broadcasting state (opponent joined, submitted, judging, result). It is **not** a separate service; it shares the same process, auth, and Postgres (DECISIONS: WS hub is part of the one Go service).
- **Postgres is the source of truth**; the hub pushes state transitions, it does not own them. Async and live therefore share one match lifecycle — live is a delivery upgrade, not a second backend.

## 9. Deployment & when-to-split triggers

**Deployment (v1) — deliberately boring:**
- **One Go binary** (modular monolith) — serves the JSON API and (later) WS on one port.
- **One Postgres.**
- **Static frontend** — `apps/web` built to static assets, served by CDN/static host (or by the Go binary in the simplest setup).
- **Object storage** for rendered PNGs (thumbnails + judged rasters) — **added when needed**, not day one.
- **Render worker** — a small **Node** sidecar importing `packages/document` to rasterize submissions authoritatively (one renderer shared with the editor). Inline/synchronous first; a queue only if it becomes a bottleneck.
- **External judge** — the collaborator's HTTP service; we point `HTTPJudge` at it via config, fall back to `FakeJudge` otherwise.

**Split only when a trigger actually fires** (resist premature distribution — microservices are a portfolio anti-pattern here):

| Trigger (must be observed, not anticipated) | Split to consider |
|---|---|
| Render worker saturates the API process / blocks request latency | Extract render worker to its own service + a job queue |
| The game earns its own brand/audience | Split `/play` from `/draw` into a separate app (the `editor` package makes this mechanical — DECISIONS calls it reversible) |
| WS connection count / fan-out outgrows one process | Lift `internal/ws` into a dedicated realtime service behind its interface |
| A module needs independent deploy cadence or scaling that hurts the monolith | Lift that module out behind its existing Go interface |
| `editor`/`document` wanted by a third party or a second product | Publish them as versioned npm packages (still one repo, just released) |
| Postgres becomes the bottleneck for a specific access pattern | Add a cache or read replica before sharding/splitting the DB |

Until a trigger fires, the answer is **one binary, one DB, one repo**. The internal seams (module interfaces, package boundaries, the Judge interface, the render-from-document path) are precisely what make each split mechanical *later* — so we get the option value without paying the distribution tax now.
