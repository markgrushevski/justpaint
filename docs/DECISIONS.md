# Decision log

Lightweight record of key decisions and their rationale, so they aren't relitigated and survive context resets / onboard new agents and collaborators. Newest first.

## 2026-06-20 — Deferred hardening (backend review)

A multi-agent adversarial review of the Go backend (29 confirmed findings) drove a batch of fixes; two items are **intentionally deferred** — they are not Phase-1 deliverables and are unreachable or low-risk at the current greenfield stage:

- **Rate limiting (`429 rate_limited`).** `web.CodeRateLimited` is reserved and `docs/API.md` advertises 429 on auth + write routes, but no limiter ships in Phase 1. Add a per-IP / per-login limiter (or enforce at the edge/reverse proxy) before any public deploy. The implemented anti-enumeration (generic `invalid_credentials` + dummy-hash timing) stands without it.
- **Duel-submission immutability (`409 conflict`).** PUT/DELETE on a submitted duel drawing must return 409 (`API.md` §7), enforced once the game/submit path (Phase 3) can create match-linked drawings. Unreachable today — `drawings` create always sets `match_id = null`.

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
