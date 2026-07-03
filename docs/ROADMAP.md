# Roadmap & status

> **Dual purpose.** The phased plan **and** the durable status tracker. This file survives context resets — it is the source of truth for *what's done, what's next, and why*. Update the status markers as work lands; don't keep status in an ephemeral task list.
>
> Companion docs: `docs/DECISIONS.md` (the "why"), `docs/DOCUMENT-FORMAT.md` (the keystone schema), `docs/ARCHITECTURE.md` (topology & boundaries). When this disagrees with reality, fix this file.

## Status at a glance

| Phase | Goal | Status |
|---|---|---|
| **0** | Specs — agree the contracts before code | 🟢 **done** |
| **1** | Go backend (auth + drawings CRUD) + minimal Konva editor | 🟢 **done** |
| **2** | Frontend refactor — vector editor, real layers; oriui swap | 🟢 **done** |
| **3** | Game — async duel first, then live WS | 🟡 **in progress** |
| **4** | Stretch — realtime hub, ratings, teams/tournaments, replay | ⚪ not started |

Legend: ⚪ not started · 🟡 in progress · 🟢 done. Within a phase, check off deliverables as they land.

**North star** (per `CLAUDE.md` / `DECISIONS.md`): the AI-judged drawing duel. Every phase is sequenced to reach a playable async duel (Phase 3) as fast as the foundations allow; `/draw` is the supporting mode that falls out for near-free. Don't build two products.

---

## Phase 0 — Specs 🟢 (done)

**Goal:** lock the contracts the rest of the build depends on, so Phases 1–3 don't relitigate them mid-flight. The keystone is the vector document format (shared by editor, backend, judge).

**Deliverables**
- [x] `docs/DECISIONS.md` — decision log.
- [x] `docs/DOCUMENT-FORMAT.md` — the keystone vector-doc schema (v1).
- [x] `docs/ROADMAP.md` — this file (phases + durable status).
- [x] `docs/ARCHITECTURE.md` — monorepo topology, package boundaries, request/render/trust flows.
- [x] `docs/API.md` — HTTP contract + WS sketch: routes, `jp_session` cookie, `{error:{code,message}}` envelope + status map, DoS caps, pagination.
- [x] `docs/JUDGE.md` — the `Judge` contract (`prompt + 2 PNGs → {scoreA, scoreB, winner:"A"|"B"|"tie", reason}`), 1024² opaque-white raster, FakeJudge/HTTPJudge, versioning. Single owner of the winner/tie/raster definition.
- [x] `docs/GAME.md` — match lifecycle (open→drawing→judging→done/abandoned), square 1080² canvas, prompt pinning, trust boundary, Elo sketch (K=32, tie=0.5).

**Exit criteria**
- Document format v1 is frozen enough to code against (TS canonical types + Go validator can be written from it without further decisions).
- Judge contract pinned: raster size, background, **and** the `winner` representation (string `"A"|"B"|"tie"` vs resolved player id) — one definition the other docs cite.
- API shape (routes, auth, limits) decided well enough to implement Phase 1 without inventing it ad hoc.

---

## Phase 1 — Go backend + minimal editor 🟢 (done)

**Goal:** a secure Go service that authenticates users and does drawings CRUD storing the **vector document as jsonb**, plus the thinnest Konva editor that can produce and load a valid document. First real learn-Go surface.

**Deliverables**
- [x] **Monorepo skeleton** — npm-workspace root (`package.json` workspaces `packages/*`+`apps/*`, shared strict `tsconfig.base.json`); members: `packages/document`, `apps/web` (migrated from `client/` via `git mv`, history preserved, renamed `@justpaint/web`); `server/` already at target. oriui was `file:`-linked from a temporary local `vendor/oriui/` copy (alpha.1), later swapped to the published npm packages in Phase 2 (`@oriui/* 1.0.0-alpha.2`; see `IDEAS.md`). `npm install` + `vite build` + `vue-tsc` green; internals refactor is Phase 2.
- [x] **`packages/document` v1** — canonical TS types (§3–§5 of `DOCUMENT-FORMAT.md`); `parseDocument`/`serializeDocument` (write-precision rounding, §2); `validateDocument` mirroring the Go validator (every invariant + DoS caps, single id namespace); the pinned `computeFitTransform` (contain, §10) + `toFreehandOptions` render-contract constants + `FREEHAND_VERSION` pin (§5.3/§9). 40 Vitest tests green (validate table ported 1:1 from the Go tests, serialize/parse round-trip, fit); `tsc` + `build` (dist + d.ts) clean. The `renderToPNG`/`toKonva` **Konva** seam lands with `packages/editor` — kept out of the pure, dependency-free contract package so anything (incl. the future Node render worker) can import types/validation without pulling Konva.
- [x] **Go service scaffold** — net/http + `ServeMux` (1.22 method patterns) + slog, `internal/platform/{config,logging}`, env config with **fail-fast on `JWT_SECRET`** (no empty-secret fallback), graceful shutdown, `/healthz`. `go build` + `go vet` + gofmt + runtime smoke all green. Old NestJS `server/` removed (recoverable from git history).
- [x] **Auth** — `internal/auth` (service/handler/middleware/token/password): bcrypt (+SHA-256 prehash for long passwords), JWT HS256 with alg-pinning, `jp_session` cookie (HttpOnly/SameSite=Lax; Secure in prod, off for dev http); `register/login/logout/me` + `RequireAuth` middleware; anti-enumeration (generic `invalid_credentials` + dummy-hash compare). Verified end-to-end via curl against Postgres (201/200/401/409/400/204 per API.md). Shared `internal/platform/web` envelope + strict JSON decode.
- [x] **DB** — Postgres via Docker (`docker-compose.yml`) + pgx pool (`internal/platform/postgres`); **goose** migration `00001_initial_schema` (users/prompts/matches/drawings/match_players per ARCHITECTURE §7 + DOCUMENT-FORMAT §7); **sqlc** type-safe queries (uuid→string, timestamptz→time.Time, nullable→pointers, `drawings.document`→`json.RawMessage`). Migrated + generated + builds green.
- [x] **Drawings CRUD** — `internal/drawings` create/get/list/update/delete + `internal/document` write-edge validator (discriminated-union `Stroke` decode via `UnmarshalJSON`, every invariant, DoS caps, 8 MB `http.MaxBytesReader`→413). Ownership-scoped (foreign→404, no IDOR); keyset pagination (opaque cursor) with free/duel/all filter. Verified end-to-end.
- [x] **Minimal Konva editor + round-trip** — `packages/editor` (`@justpaint/editor`): full pure `buildStroke` tool set + `toKonva`/`renderToPNG` + `Editor` controller (logical coords, `Editor.destroy()` on unmount); 14 tests; `FREEHAND_VERSION` pinned to perfect-freehand 1.2.3. **Wired into `apps/web`**: a vue-router `/draw` page mounts the `Editor` with an oriui toolbar (tools, color, width, fill, New, Export PNG, Save/Load) + a `SessionBar` (login/register/logout). Auth + drawings go through a **native-`fetch`** client (`src/core/api/drawings.ts`, `credentials:'include'`, no axios) + `useSessionStore`; a vite dev proxy forwards same-origin `/api` → the Go server so the `jp_session` cookie is first-party. **Round-trip verified live end-to-end** through the real UI: register/login → draw → save → reload (session restored) → load → the **same drawing back**. `vue-tsc` + `vite build` green; preview via `.claude/launch.json`.
- [x] **Tests** — Go validator table tests (valid full document + every rejection path + union decode, `internal/document`). **`packages/document`** 40 Vitest (validate table mirrored from Go, serialize/parse round-trip + write-precision, fit). **`packages/editor`** 14 Vitest (each tool's `buildStroke` validated against `@justpaint/document`). **App round-trip verified live** (register→draw→save→reload→load through the UI + Go + Postgres); an automated e2e harness is a later nicety, not blocking.

**Exit criteria**
- A user can register, log in (cookie-based), draw with every tool, save, reload the page, and get the **same drawing back** — round-tripped as a vector document through Postgres jsonb.
- Invalid/oversized/forged documents are rejected by the Go validator with clear errors.
- No plaintext passwords, no empty-JWT-secret fallback, no token in localStorage (the old red flags are structurally gone).

**✅ All exit criteria met** — the full register → draw → save → reload → load round-trip is verified live (real UI + Go + Postgres jsonb); the drawings/auth client is native `fetch`.

---

## Phase 2 — Frontend refactor (vector editor + real layers) 🟢 (done)

**Goal:** turn the minimal editor into the real one — a proper vector editor with **real layers**, command-based undo/redo, and clean export — extracted into `packages/editor` so both `/draw` and `/play` consume it. The **oriui swap is a separate, isolated pass** (don't entangle it with the editor rewrite).

**Deliverables**
- [x] **`packages/editor`** — the real editor, built across `feat/editor-history-layers` + `feat/editor-fit`: `@justpaint/editor` has the pure tool set, `toKonva`/`renderToPNG`, the `Editor` controller, a command-based `History`, real layer operations, an `onChange` subscription seam (host reads `getLayers()`/`canUndo()`/`getZoom()` — no Vue in the package), and a `view` seam (fit-to-viewport, zoom, pan — scales the Konva *stage*, not CSS). Depends only on `packages/document` + Konva + perfect-freehand (ARCHITECTURE §3).
- [x] **Real layers** — ordered list with id/name/visible/opacity/z-order, mapped to `Konva.Layer` (own `<canvas>`, per-layer isolation); add/remove/reorder/rename/visible/opacity via the `Editor` + a `/draw` layers panel. Replaces the old fake-layer + center-anchored PNG compositing. *(feat/editor-history-layers)*
- [x] **Command-based undo/redo** — a command stack over the `Document` keyed by stroke/layer `id` (add-stroke/add/remove/reorder/rename/visible/opacity), with `Ctrl/Cmd+Z` / `Shift+Z` / `Ctrl+Y`. **Replaces PNG-snapshot history entirely.** History is runtime-only; never persisted in jsonb. *(feat/editor-history-layers)*
- [x] **perfect-freehand brush** — store input points + curated brush options (store-input-not-output, §5.3); `FREEHAND_VERSION` pins the resolved perfect-freehand (render contract). *(Phase 1 `packages/editor` / document)*
- [x] **Clean export** — `document → PNG` via the shared `renderToPNG` (pinned contain-fit); Export button on `/draw`. **Server-side thumbnails** (`drawings.thumbnail_url`) wait on the Node render worker — moved to **Phase 3** (the trust boundary needs the authoritative raster rendered off the client anyway).
- [x] **`/draw` page** — editor + save/load only, on the oriui design system with fit/zoom; kept deliberately minimal (no feature creep).
- [x] **State** — session in Pinia (`useSessionStore`); server data via **TanStack Query** (`useSaveDrawing`/`useLoadLatestDrawing` mutations, `feat/web-query`); the editor owns its own document/view state in `packages/editor`. The ad-hoc axios path is gone (legacy removed).
- [x] **oriui swap (isolated pass)** — DONE EARLY (ahead of Phase 1): replaced `vueinjar` (`VButton/VCard/VIcon/VAvatar` across 14 files) with **oriui** (`@oriui/vue` + `@oriui/css`); `npm run types` + `vite build` green. Kept separate from the editor rewrite. **Vendored→npm swap landed 2026-07-02** (`build/oriui-npm-swap`): `@oriui/{vue,css,headless}` `1.0.0-alpha.2` from the registry; `vendor/oriui/` deleted.

**Exit criteria**
- `/draw` is a usable vector editor: real layers (reorder/toggle/opacity), undo/redo via commands, save/load, PNG export, fit/zoom — all on the v1 document format.
- `packages/editor` is consumed by `apps/web` with no editor logic left in the app shell.
- No `vueinjar` imports remain; UI runs on oriui.

**✅ All exit criteria met** — `/draw` is a real vector editor (layers, command undo/redo, fit/zoom, save/load via TanStack Query, PNG export) on the oriui design system; the editor lives entirely in `packages/editor`. Deferred to Phase 3: **server-side thumbnails** (need the Node render worker + object storage) and **touch pinch/pan** polish (IDEAS).

---

## Phase 3 — Game (async duel first, then live) 🟡 (in progress)

**Goal:** the north star, shipped. **Async duel first** — the simplest complete loop — then layer live realtime on top. The editor from Phase 2 powers both duelists.

**Deliverables**
- [ ] **`docs/GAME.md` realized** — match lifecycle in Go: create match → both players draw the same prompt → submit → judge → result; canonical game canvas size enforced.
- [ ] **Prompts** — prompt source/table; a match pins one prompt for both players.
- [ ] **Matches** — `matches` table + state machine (open → drawing → judging → done → abandoned); each drawing links to its match (`match_id` column, format §7); two `match_players` rows per 1v1.
- [ ] **Submit** — server renders the **authoritative judged raster from the vector document, off the player's machine** (Node render worker sharing `packages/document`, per-layer-isolation algorithm). Client PNGs are advisory only; the vector doc is the source of truth (trust boundary, format §10).
- [~] **Judge seam + fake impl** — `internal/judge` **landed** (`feat/game-judge`): the `Judge` interface (`Score(ctx, Request{Prompt, ImageA, ImageB []byte}) → Result{ScoreA, ScoreB, Winner, Reason}`), a deterministic in-process **`FakeJudge`** (ink-coverage heuristic, tie within epsilon), and `Result.Validate` enforcing the JUDGE.md §2 contract; table tests (91% cov), reviewed PASS by jp-go + jp-scope-guard. **Remaining:** the positional `winner` → player-id mapping (lands with the game/submit module below); `HTTPJudge` (the real ML wiring) is **Phase 4**. Never block on the collaborator's judge.
- [ ] **`/play` page** — create/join an async match, draw, submit, see result + reason. Reuses `packages/editor`.
- [ ] **Object storage seam** — rendered judged PNGs (and thumbnails) to object storage (URLs handed to the judge / stored in `thumbnail_url`); local/dev stub acceptable first.
- [ ] **Then: live realtime** — coder/websocket hub (`internal/ws`); presence + "both drawing" live state; same submit/judge tail. (Live is the *back half* of Phase 3 — async must be playable first.)

**Exit criteria**
- Two users can play an **async duel** end-to-end: same prompt → both draw → submit → fake judge scores → winner + reason shown — with the scored image rendered authoritatively server-side from the vector document.
- Swapping the fake judge for the collaborator's HTTP judge requires no schema/loop change (only the `Judge` impl).
- (Live half) two users see each other's match state in realtime over WS.

---

## Phase 4 — Stretch ⚪

**Goal:** depth once the core loop is proven. Each item is independent; pull forward whatever the portfolio needs.

**Deliverables (unordered)**
- [ ] **Realtime hardening** — WS hub robustness (reconnect, presence, match rooms), spectating.
- [ ] **Ratings** — per-player rating updated on match result (Elo-style; `users.rating`, `match_players.rating_before/after`); leaderboards.
- [ ] **Teams / tournaments** — multi-player brackets on top of the `match_players` primitive (generalizes from 1v1 without reshaping `matches`).
- [ ] **Replay / animation** — animate a drawing from its document (order-based replay works on v1; add per-point/per-stroke timing as an additive, version-safe field if true timed replay is wanted — format §9).
- [ ] **Real judge integration** — wire the collaborator's ML judge over HTTP against the live contract; keep the fake for tests/dev.

**Exit criteria:** none fixed — these are stretch. Track individually.

---

## Cross-cutting cleanups (where each lands)

Known red flags from the throwaway code (`CLAUDE.md` "known red flags"; current-code brief). These are **not** ported — they're fixed structurally by the rewrite. Tracked here so none slips through:

| Issue (old code) | Where it dies | How |
|---|---|---|
| **PNG-snapshot history** (`CanvasHistory.ts`, `drawData.*.canvasDataURL`) | Phase 2 | Command-based undo/redo over the `Document`, keyed by `id`. History is runtime-only, never in jsonb. |
| **Non-reactive / stub history** (empty `save()/load()`, manual snapshot stack) | Phase 2 | Replaced by the command stack; no more PNG frames or empty stubs. |
| **DPR / CSS-pixel coords** (`pageX - offsetLeft`, `CanvasTool.ts:257-269`) | Phase 1 | Capture in logical/stage coords (`getRelativePointerPosition`); document stores DPR-independent logical units (format §2). |
| **Triangle draws a rectangle** (`CanvasTool.ts:386-396`) | Phase 1 | Modeled as `polygon` (closed 3-point); triangle bug is structurally impossible (no `Konva.Triangle`). |
| **"Circle" `/√2` bbox quirk** (`CanvasTool.ts:363-364`) | Phase 1 | Standard `ellipse` (center + radii) from drag bbox. |
| **"Square" is a free rectangle** | Phase 1 | Renamed `rect`; a constrained square is `width === height` (editor concern). |
| **Eraser modeled inconsistently across tools** | Phase 1 | The old eraser was already correct (`destination-out`, `CanvasTool.ts:309-323`); we generalize it to a per-stroke `composite` field shared by all stroke types — not a bug fix, a model unification. |
| **Dev/prod default-color divergence** (`CanvasTool.ts:69-77`) | Phase 1 | Defaults live in editor/tool config, identical everywhere; the document always carries explicit resolved colors. |
| **bytea / base64-PNG-per-layer storage** (`layer.entity.ts`, `arts.dto.ts`) | Phase 1 | Vector document as jsonb; PNG is a derived/cached artifact only (`thumbnail_url`). |
| **Plaintext passwords** | Phase 1 | bcrypt. |
| **JWT empty-secret fallback** | Phase 1 | Mandatory secret; fail fast if unset. |
| **Token in localStorage** | Phase 1 | httpOnly + secure + sameSite cookie. |
| **Broken axios error handling** | Phase 2 | TanStack Query + a single typed API client with a consistent error path. |
| **Client-side max-size + center-anchor compositing** (`getCompositedArts.ts`) | Phase 2 | One shared coordinate space + z-order/opacity; one renderer (`packages/document`) with per-layer isolation. |
| **Dead code / empty stubs** (`CanvasLayers.ts`, `width?/height?` "after compositing" fields) | Phase 1–2 | Superseded by the schema; not carried into the rewrite. |
| **NestJS backend** | Phase 1 | Replaced wholesale by Go — not refactored. |

---

## Working notes

- **Order is load-bearing:** format (Phase 0) → storage + validator + editor (Phase 1) → real editor (Phase 2) → game (Phase 3). Don't start the game loop before the document round-trips and the editor is real.
- **Never block on the ML judge.** The fake impl keeps Phase 3 fully playable; real judge is Phase 4.
- **Keep `/draw` minimal.** It exists to exercise the editor and host save/load — not to grow into a second product.
- **Update this file when a deliverable lands or a phase flips status.** It's the durable tracker; treat a stale ROADMAP as a bug.
