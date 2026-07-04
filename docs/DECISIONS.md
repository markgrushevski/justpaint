# Decision log

Lightweight record of key decisions and their rationale, so they aren't relitigated and survive context resets / onboard new agents and collaborators. Newest first.

## 2026-07-04 — UX-first: polish /draw + shell before the /play UI

The owner directed: improve the site's and `/draw`'s UI/UX **before** building the game-mode UI. The game *backend* (async duel loop + authoritative render) is done and unaffected — this re-sequences only the frontend work: the UX pass now gates the `/play` page (tracked in `ROADMAP.md` Phase 3).

**"Keep `/draw` minimal" is reinterpreted:** still *focused* — editor + save/load, no feature creep — but **polished**: proper oriui layout/spacing (the toolbar is currently cramped, no gaps/margins), the legacy **slide-in side menu** pattern returns (hidden by default, opened by a toggle button; auth + profile live inside it — the interface the owner specifically liked, see `IDEAS.md` "Salvaged from the `/legacy` app"), and canvas interaction correctness (no drawing outside the document area, immediate eraser feedback).

Also declared: the **AI-in-product** direction (see `IDEAS.md` "AI inside the product") as a planned north-star upgrade — AI features inside the product itself; priority high, sequencing vs `/play` TBD.

## 2026-07-03 — The authoritative render worker (Phase 3)

Realizing the `render.Renderer` seam with the real thing (`feat/render-worker`): a headless Node worker that renders the vector document to the judged raster off the client.

### One shared renderer — the worker reuses the editor's `renderToStage`
The judged raster must be produced by the **same** Konva + perfect-freehand path the editor draws with (why `FREEHAND_VERSION` is pinned) — a second renderer (e.g. a Go rasterizer) would silently diverge. So the worker imports `@justpaint/editor`'s projection, not a reimplementation. To make that path headless-safe, `toKonva`/`renderToPNG` were split: a DOM-free **`renderToStage`** (container guarded by `typeof document`) is the shared core; `renderToPNG` (browser, `toBlob`) and the worker (`stage.toDataURL` under `konva/canvas-backend`) are thin output tails over it. The browser path is byte-identical (verified: `/draw` export still yields a valid PNG); the worker renders the same pixels headless (verified live).

### Stack: node-canvas + Konva 10 (`konva/canvas-backend`), viable on Windows
A feasibility spike confirmed `canvas` (node-canvas) installs from a prebuild (no node-gyp) on Windows + Node 24, and Konva 10 renders headless once `konva/canvas-backend` is imported **before** any Konva use (Konva 10 dropped the default Node backend). node-canvas is now a real repo dependency (native) — documented in NOTES; a fresh `npm install` builds/fetches it.

### The worker is esbuild-bundled (the packages aren't Node-ESM-loadable)
`@justpaint/{editor,document}` emit **extensionless** relative imports (`./ids`) — fine for Vite/the browser app, but native Node ESM refuses them. Rather than churn `.js` extensions across the frozen contract package + editor (and risk the mirror), the worker is **bundled with esbuild** (`packages/render/dist/render.mjs`), inlining the workspace packages and keeping `canvas` external (native). Isolated to one new package; zero change to document/editor source.

### Go ⇄ Node: spawn-per-render, document JSON in / base64 PNG out
`render.NodeRenderer` (`server/internal/render`) shells out (`exec.CommandContext`) to `node dist/render.mjs`, piping the document JSON on stdin and reading the PNG **base64** on stdout (base64 keeps the pipe text-safe cross-platform). Spawn-per-render (not a long-lived service or queue) is right-sized for v1 async judging (two renders/match, off the request path); a resident worker is a later optimization. The `Renderer` interface stays `Render(ctx, document.Document)`; `NodeRenderer` re-marshals the validated doc to JSON (unknown fields are irrelevant to rendering).

### `RENDER_MODE` config — stub default, node opt-in (fail-fast)
`RENDER_MODE=stub` (default) keeps the zero-dependency in-process stub so the Go server runs with no Node/canvas present (dev, CI, tests). `RENDER_MODE=node` selects the authoritative worker and **requires `RENDER_CLI`** (path to the bundle) — the server fails fast at boot if it's unset or the mode is unknown, matching the `JWT_SECRET`/`DATABASE_URL` fail-fast posture. This keeps "correct by default to run" and "authoritative when configured" both true, and makes the seam a one-env-var swap.

## 2026-07-03 — Submit, judging & the render seam (Phase 3, loop closed)

Closing the async-duel loop (`internal/game`: `POST /api/matches/{id}/submit`, out-of-band judging, `GET …/result`). The lifecycle now runs `open → drawing → judging → done` end-to-end (verified live).

### The renderer is a seam, exactly like the judge (`render.Renderer` + `StubRenderer`)
The authoritative judged raster must be rendered **off the client** from the vector document (trust boundary, `GAME.md` §6). It must also match the editor's output pixel-for-pixel — which is why `FREEHAND_VERSION` is pinned — so the real renderer has to be the **same** Konva + perfect-freehand path the editor uses, i.e. a **Node** worker (native `canvas`), not a reimplementation in Go (a Go rasterizer would silently diverge). Rather than block the loop on standing up that worker (native deps, a new deployable), we mirror the Judge seam: a Go `render.Renderer` interface with an in-process **`StubRenderer`** (deterministic 1024² PNG, ink coverage ∝ stroke count). The whole submit → render → judge → result loop runs today; the real Node worker swaps in behind `Renderer` with **no loop change** — the same guarantee we already have for `Judge`. The stub is loop-proving, not pixel-authoritative (`NOTES.md`).

### Judging is out-of-band (in-process goroutine), per API.md §8.3
Submit returns **202** immediately; the verdict is produced asynchronously. On the final submit the handler flips the match to `judging` inside the submit tx, then spawns a goroutine (own `context.Background()` + timeout) that renders both docs, calls the judge, maps the positional winner → player id, applies Elo, and commits `judging → done` in one tx. **v1 accepts the durability gap:** a crash mid-judge leaves the match stuck in `judging` (no auto-retry). A restart-time sweeper that re-drives `judging` matches lands with the real render worker (`IDEAS.md`). In-process (not a queue) is right-sized for v1 — the judging work is CPU-bound and sub-second with the stub/fake.

### Last-submit detection is serialized by a match-row lock
Two players submitting at the same instant could each count the other as "not yet in" and neither would flip to `judging` (match stuck in `drawing` with both submitted). The submit tx takes `SELECT … FOR UPDATE` on the match row (`GetMatchForUpdate`), so the two submits serialize and exactly one observes "I'm last." A duel has only two players, so lock contention is negligible.

### Submit to a match you're not in is 403, not a hidden 404
The read paths hide a foreign match as 404 (no existence leak). **Submit is the deliberate exception** (`API.md` §1, §8.3): a player submitting to a match they hold an id for but aren't rostered in is a *known-ownership violation* → **403 forbidden**. (`ErrNotPlayer` on submit → 403; the same non-membership on `GET …/result` → 404, since a reader shouldn't confirm existence.)

## 2026-07-03 — Match creation & matchmaking (Phase 3, front half)

Building the async-duel front half (`internal/game`: `POST /api/matches` create/auto-join + `GET /api/matches/{id}`). API.md §8 deliberately left the *matchmaking* mechanism open ("create an open match the next caller joins, or pair immediately"); these pin the v1 choices.

### Matchmaking = open-pool auto-join (single "play" button)
`POST /api/matches` is the one entry point for both players. In one transaction it: **(1)** auto-joins the oldest waiting `open` async match the caller isn't already in — flipping it `open → drawing`; else **(2)** returns the caller's own still-open match if one exists; else **(3)** creates a fresh `open` match with one random active prompt pinned. Rationale: the simplest complete loop that's actually playable — two users each hit "play" and get matched, no separate create-vs-join UI, no invite/lobby. The route shape (`API.md` §8) is stable if we later add invite/ranked matchmaking. Concurrency is safe via `FOR UPDATE SKIP LOCKED` on the join candidate: two simultaneous joiners each grab a *different* match (the loser skips the locked row and opens its own), so a match is never double-seated (verified live).

### No duplicate open matches from one player (step 2)
A waiting player who taps "play" again gets their **existing** open match back, not a second one (one extra `FindMyOpenMatch` query on the miss path). This dedupes the common **sequential** re-tap. It is **best-effort, not serialized**: under the default Read-Committed isolation, two *truly concurrent* creates from the same not-yet-seated user each miss the other's uncommitted row and can still open two matches. That is benign self-clutter — never a double-*seat* (the `match_players` composite PK guarantees that) — and acceptable for v1. The hard fix (a `pg_advisory_xact_lock(userID)` at the top of the create tx, or a partial unique index) is folded into the deferred rate-limit slice (`IDEAS.md`, same abuse class). *(jp-go + jp-security review, feat/game-matches.)*

### Prompt text is redacted until the match leaves `open`
The pinned prompt's `text` is withheld while `status = open` (only `id` is sent); it is revealed once the match enters `drawing`. This enforces `GAME.md` §5 fairness — a creator waiting alone must not be able to pre-draw before the opponent joins; both get the prompt at effectively the same moment (the joiner's create response is already `drawing`+text; the creator sees it on their next poll). **This overrode the `API.md` §8 example**, which mistakenly showed `text` in an `open` response — GAME.md owns reveal timing, so API.md was corrected to match.

### Opponent identity: userId + optional displayName only — never `login`
The roster query deliberately does **not** select `users.login`. `login` may be an email (identity is email-*or*-nickname, 2026-06-19 decision); exposing it to the opponent would leak PII. Players are shown `userId` + nullable `displayName`; a null name is the client's problem to label ("Player 2"), not a reason to fall back to `login`.

## 2026-06-20 — Deferred hardening (backend review)

A multi-agent adversarial review of the Go backend (29 confirmed findings) drove a batch of fixes; two items were **intentionally deferred** — not Phase-1 deliverables, unreachable or low-risk at the greenfield stage. One (duel immutability) has since landed; rate limiting remains deferred:

- **Rate limiting (`429 rate_limited`).** `web.CodeRateLimited` is reserved and `docs/API.md` advertises 429 on auth + write routes, but no limiter ships in Phase 1. Add a per-IP / per-login limiter (or enforce at the edge/reverse proxy) before any public deploy. The implemented anti-enumeration (generic `invalid_credentials` + dummy-hash timing) stands without it.
- **Duel-submission immutability (`409 conflict`).** ✅ **RESOLVED 2026-07-03** (`feat/game-submit`, caught by the jp-security lens). The defer was gated on "unreachable while `drawings` create always sets `match_id = null`" — the submit path fired that precondition by creating `match_id`-set drawings, so the guard **had** to land with it: `UpdateDrawing`/`DeleteDrawing` now carry `and match_id is null`, and the drawings service classifies the resulting no-op as `ErrDuelLocked` → **409** (vs 404 for a genuinely absent/foreign row). Without it, a player could `PUT` a better document over their submission before the opponent submitted, and judging would render the swapped doc — breaking the trust boundary + Elo fairness.

## 2026-06-19 — Phase 0 contract resolutions

Resolving the open questions the Phase 0 specs surfaced.

### Square canvas + square judge frame
The game canvas is **square 1080×1080**; the judge renders a **square 1024×1024** frame. Both duelists share this canonical square. Rationale: letterbox bars count as judged pixels and skew similarity — square↔square avoids them. The general free-draw document default stays 1920×1080 (DOCUMENT-FORMAT §2); only the **game** pins square (owned by `docs/GAME.md`).

### Game screen visibility
During a round each player sees only their OWN canvas; both canvases are revealed on the end-of-round result screen. A `docs/GAME.md` detail (informs the WS/state design, not the document schema).

### Ties are allowed
The judge may return `winner: "tie"`; `matches.winner_player_id` is therefore nullable. Tie handling for ratings (e.g. shared/half points) is a `docs/GAME.md` detail.

### Auth: login (email or nickname) + password, optional display name
Identity is a single **`login`** credential that may be an email OR a nickname, plus a password. **`display_name`** is optional. Supersedes the earlier ARCHITECTURE sketch's email-only `email citext` field. Exact rules (format validation, case-folding) owned by `docs/API.md` / Phase 1.

### Document size / DoS caps
The binding limit is **total input points**. Computed acceptable v1 values (tunable; pinned authoritatively in `docs/API.md`):

| Limit | Value | Why |
|---|---|---|
| Request body (`http.MaxBytesReader`) | 8 MB | Outer DoS guard. A max legit doc (~100k points ≈ 2.5–3 MB jsonb) sits well under it, so the semantic points cap — not the byte cap — trips first on real drawings. |
| Total input points (all strokes) | 100,000 | A frantic ~5-min duel ≈ 30k points; 100k gives generous headroom and still rasterizes sub-second at 1024². The binding semantic cap. |
| Points per single stroke | 10,000 | Bounds one pathological stroke. |
| Total strokes | 5,000 | The points cap binds first; this is a sanity ceiling. |
| Layers | 64 | Far above any hand-editor need. |

## 2026-06-19 — Foundational decisions

### Game is the north star; editor is supporting
The centerpiece is an AI-judged drawing duel, not the paint editor. The standalone editor (`/draw`) is a supporting mode, kept minimal. Rationale: a generic paint app is a commodity portfolio project; the AI-judged game is the memorable differentiator. Don't build two products.

### Two modes in one site (not only-game, not separate sites)
`apps/web` serves `/draw` (free) + `/play` (game). The editor exists for the game anyway, so `/draw` is near-free and useful (dev surface, home for save/load, fallback demo). Separate sites = premature ops/focus split for a solo project; the reusability signal already comes from the `editor` package boundary. Reversible later if the game earns its own brand.

### Use libraries for canvas; don't hand-write an engine
Rendering on **Konva** + **perfect-freehand**; we own only the document model/serialization and a thin editor wrapper. Rationale: a custom render engine reinvents Konva/Fabric, doesn't advance the learn-Go goal, and isn't the differentiator. Own the boundaries, rent the rendering. **Konva over Fabric** — explicit layers, speed, JSON serialization, `vue-konva`, clean PNG export for the judge.

### Backend rewritten in Go as a modular monolith
Replace NestJS with one Go service (auth + drawings + game + WS hub), one Postgres. Stack: net/http + pgx + sqlc + goose + golang-jwt + bcrypt + slog + coder/websocket. Rationale: learn-Go is an explicit goal; a monolith is right-sized for solo; microservices would be a portfolio anti-pattern. Don't refactor the old NestJS — replace it.

### Vector document persisted as jsonb
Drawings stored as a structured vector document (`Document/Layer/Stroke`), not bytea PNG. Enables small storage, clean export, real layers, replay, and a clean contract for the judge. The schema is independent of Konva's internal JSON to avoid vendor lock.

### The ML judge is external (built by a collaborator)
A friend builds the ML judge (his own portfolio piece). We define the `Judge` interface (prompt + 2 images → `{scoreA, scoreB, winner, reason}`) and ship a fake impl; he integrates over HTTP against the live contract. Never block on the ML.

### Greenfield
No production data to preserve; free to redesign schema/format. (If this ever changes, revisit migration strategy.)

### Component library: oriui (replacing vueinjar)
Frontend uses the owner's own component library **oriui** (alpha, on npm) instead of `vueinjar`. Dogfooding. Current `vueinjar` surface to migrate: `VButton`, `VCard`, `VIcon`, `VAvatar` across ~12 components.
