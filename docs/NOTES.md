# Implementation notes

A running log of **non-obvious implementation nuances and gotchas** — things that cost real time to
work out but don't rise to an architectural decision. **Read this first; append what you discover**
so the next person (or agent) doesn't re-analyze it.

Companion to [`CLAUDE.md`](../CLAUDE.md) (conventions), [DECISIONS.md](DECISIONS.md) (rationale /
ADRs), and [REVIEW.md](REVIEW.md) (the per-change bar). Architectural decisions go in DECISIONS.md;
small practical gotchas go here.

## Document contract & TS↔Go parity

- **The format has two homes that must stay 1:1** — `packages/document` (TS) and
  `server/internal/document` (Go). The TS `validate` test table was ported from the Go tests; treat
  them as a mirror. A rule added to one side without the other is a latent bug, not a nuance.
- **Color regex is lowercase-only** (`^#([0-9a-f]{6}|[0-9a-f]{8})$`). `#FFFFFF` is **rejected** — the
  TS serializer must lowercase before POST or the write 400s.
- **Point arity is enforced by custom `UnmarshalJSON`** on `FreehandPoint` (len 3) and `Point`
  (len 2), because `encoding/json` silently zero-fills short fixed arrays and drops extras. If you
  ever change those array types, re-add the arity guard or malformed points sneak through.
- **The `Stroke` union is a sealed interface** in Go (`base()` is unexported) — you can't add a
  stroke type from outside the package. A new type needs edits in **three** Go sites (the struct +
  const in `document.go`, the switch in `parse.go`/`unmarshalStroke`, the switch in
  `validate.go`/`checkStroke`) plus the TS mirror.
- **`FREEHAND_VERSION` (`packages/document/src/constants.ts`) must equal the RESOLVED installed
  perfect-freehand.** `^1.2.0` resolves to 1.2.3 — the pin is the exact resolved string, not the
  range floor. perfect-freehand is a dep of `packages/editor` only (not `apps/web`). Bump the
  constant in lockstep or rendered outlines diverge between the editor preview and the (future)
  server render worker.
- **Write-precision rounding lives only in `serializeDocument`** (2dp geometry, 3dp pressure). Don't
  round in the model or in tools — repeated rounding accumulates error.
- **Ids share a single namespace** across layers + strokes (one `seen` set); a stroke id colliding
  with a layer id is rejected.

## Go / pgx / sqlc / goose

- **The server does NOT auto-load `.env`.** `server/.env.example` exists but there's no
  godotenv/embed — `DATABASE_URL` + `JWT_SECRET` must be in the process environment or `config.Load()`
  fails fast. Export them (or use an IDE run config); copying `.env` alone does nothing.
- **`go.mod` targets `go 1.26`** (toolchain go1.26.4). The ROADMAP's "ServeMux (1.22 method patterns)"
  note is about the routing feature's origin, not the module version — you need Go ≥ 1.26 to build.
- **`docker-compose.yml` is at the REPO ROOT**, not under `server/`. Run `docker compose up -d` from
  the root (postgres:17-alpine; db/user/pass all `justpaint`; :5432).
- **goose and sqlc are external CLIs**, not Go deps and not wired into any Make target (there is no
  Makefile). Migrations run via the goose CLI against `server/migrations/`; regenerate query code with
  `sqlc generate` (`server/sqlc.yaml`; generator output pinned to sqlc v1.31.1).
- **`CookieSecure` is derived from `ENV` alone** (default `dev` → Secure=false). Deploying with `ENV`
  unset ships non-Secure cookies — set `ENV=prod` (which also requires `JWT_SECRET` ≥ 32 bytes or the
  server refuses to boot).
- **Migration 00001 creates `prompts` / `matches` / `match_players`; 00002 seeds `prompts`** (24 active
  starter prompts). `internal/game` now queries all three (create/auto-join/get); `match_players` is
  populated but `drawing_id`/`score`/`submitted_at` stay null until the submit slice. Retire a prompt
  with `active = false`, never a delete (matches FK-reference `prompt_id` forever).
- **Multi-write game paths run in an explicit pgx transaction** (`pool.Begin` → `s.q.WithTx(tx)` →
  `defer tx.Rollback` (no-op after commit) → `tx.Commit`). The drawings CRUD is single-statement so it
  uses `*db.Queries` directly; the game service holds **both** the `*pgxpool.Pool` (to begin the tx) and
  `*db.Queries` (for read paths). `assemble()` takes a `*db.Queries` so it works for either — the tx
  handle during create/join, the shared pool during a read. **Caveat:** a `Get` therefore runs its
  three reads (match → prompt → roster) on the pool with **no snapshot**, so a concurrent write landing
  mid-read could yield a torn view (e.g. `status = open` but a 2-player roster). Harmless while
  `CreateOrJoin` is the only writer; the **submit slice should wrap `Get` in a read-only `pgx.Tx`** (or
  collapse it to one joined query) once there are more writers.
- **Open-match anti-stacking is Read-Committed best-effort, not a hard constraint** (`DECISIONS.md`
  2026-07-03). `CreateOrJoin` dedupes a user's own open matches with a plain `SELECT` (`FindMyOpenMatch`),
  which two *concurrent* create-txs bypass (neither sees the other's uncommitted match). The composite PK
  still prevents any double-*seat*; only same-user self-stacking of `open` matches is possible, in a
  narrow window. Hard fix (advisory-xact-lock on userID / partial unique index) is folded into the
  deferred rate-limit slice (`IDEAS.md`) — don't re-file as a separate integrity bug.
- **`PickRandomActivePrompt` uses `order by random()`** — fine at seed scale (dozens of rows), but it's a
  full scan + sort; swap for a keyset/`tablesample`/id-range pick if the prompt pool ever grows large.
- **Match ownership is hidden as 404, like drawings**: a non-player calling `GET /api/matches/{id}` (or a
  non-existent / non-UUID id) gets `not_found`, never 403 — match existence must not leak (`API.md` §1).
  The player check is in the service (`isPlayer` over the loaded roster), after the row is fetched.
  **Exception — submit is 403:** `POST …/submit` by a non-player returns **403** (`ErrNotPlayer`), the one
  deliberate known-ownership case (`API.md` §8.3). Same non-membership, two codes by route — don't
  "unify" them.
- **`render.StubRenderer` is NOT pixel-authoritative** (the default `RENDER_MODE=stub`). It emits a
  deterministic 1024² PNG whose ink coverage is `0.05 + 0.02·(stroke count)` (clamped) — enough for the
  ink-coverage `FakeJudge` to produce a document-derived verdict and prove the loop, but it does **not**
  draw the actual picture. The **real** render is `RENDER_MODE=node` (below). Don't grow a Go rasterizer —
  it would diverge from the editor's output (the whole reason `FREEHAND_VERSION` is pinned).

### Render worker (`packages/render`, `RENDER_MODE=node`)

- **The worker reuses the editor's `renderToStage`** (the ONE shared projection), so the judged raster
  matches the editor preview. `renderToStage` is the DOM-free core split out of `renderToPNG`; the browser
  keeps `renderToPNG` (`toBlob`), the worker uses `stage.toDataURL()` under `konva/canvas-backend`. If you
  touch the render path, keep it going through `renderToStage` or the two surfaces drift.
- **`import 'konva/canvas-backend'` MUST be the FIRST import** in the worker, before anything that pulls
  Konva — it registers node-canvas; Konva 10 dropped the default Node backend and throws
  "unsupported environment" without it. `document` is **not** polyfilled (why `toKonva`/`renderToStage`
  guard on `typeof document`).
- **The worker is esbuild-BUNDLED** (`packages/render/dist/render.mjs`). `@justpaint/{editor,document}`
  emit **extensionless** relative imports (`./ids`) that native Node ESM refuses (`ERR_MODULE_NOT_FOUND`),
  so we bundle them in (esbuild resolves them) and keep `canvas` external (native). Consequence: **rebuild
  the worker after editing `packages/editor`** — the bundle embeds a copy (`npm run build -w
  @justpaint/render`, or the root `npm run build` fans out to it). `dist/` is gitignored, so a fresh clone
  must build before `RENDER_MODE=node` works — same dist footgun as the other packages.
- **`node-canvas` (`canvas`) is now a real, NATIVE repo dependency.** It installed from a prebuild on
  Windows + Node 24 (no node-gyp), but a fresh `npm install` fetches/builds it; a platform without a
  prebuild needs Cairo/Pango + build tools. It's isolated to `packages/render` (never bundled into the
  browser app).
- **`render.NodeRenderer` spawns `node dist/render.mjs` per render** (`exec.CommandContext`), document
  JSON on stdin → **base64** PNG on stdout (base64 dodges binary-stdout/newline munging on Windows). It
  re-marshals the validated `document.Document` to JSON (unknown fields don't affect rendering). Needs
  `node` on PATH + the built bundle + `RENDER_CLI` pointing at it (fail-fast at boot if unset).
- **`RENDER_MODE` defaults to `stub`** so the Go server runs with zero Node/canvas present (dev, CI, `go
  test`). Only `RENDER_MODE=node` requires the worker. Don't flip the default without owning the native-dep
  cost for everyone running the server.
- **Judging runs in an out-of-band in-process goroutine** (`judgeMatch`), spawned by the final submit
  *after* its tx commits, on a fresh `context.Background()` + 30s timeout (the request ctx is already
  gone). It's idempotent (`runJudging` no-ops unless the match is still `judging`) and writes the result
  in one tx. **Durability gap (v1):** a crash mid-judge leaves the match stuck in `judging` — no retry.
  A restart-time sweeper is an `IDEAS.md` item, deferred with the real render worker. Note the 30s
  budget is **stub/fake-sized**: when the real render worker + `HTTPJudge` (10s + retries, `JUDGE.md` §7)
  land, widen it so the outer context outlives render + judge-with-backoff. Also: the 30s judging window
  can outlive the 10s graceful-shutdown timeout, so a clean shutdown can still abort a pass (the sweeper
  is the fix; a `sync.WaitGroup` waited during shutdown is the natural interim).
- **`runJudging`'s status check is a no-op guard, NOT mutual exclusion — the real lock is in
  `persistResult`.** `runJudging` reads status on the pool (unlocked) and bails if not `judging`; that
  alone wouldn't stop two concurrent passes both rendering then both writing. So `persistResult` re-takes
  `GetMatchForUpdate` and re-checks `status == judging` inside its write tx — whoever locks first commits
  `done`, the loser bails. This is what makes the planned restart-sweeper safe to run alongside the live
  trigger (idempotent, never double-applies Elo). Keep that lock if you refactor judging.
- **A submitted duel drawing is immutable via CRUD.** `UpdateDrawing`/`DeleteDrawing` carry
  `and match_id is null`, so `PUT`/`DELETE /api/drawings/{id}` on a match-linked drawing touches no row;
  the drawings service (`classifyMiss`) turns that miss into `ErrDuelLocked` → **409** (a genuinely
  absent/foreign id stays 404). Without this a player could swap in a better document after submitting
  (before the opponent does) and judging would render the swap — trust-boundary + Elo break. `match_id`
  is fixed at creation, so the classify-on-miss is race-free. (Landed `feat/game-submit`; jp-security.)
- **The final-submit → judging flip is serialized by `GetMatchForUpdate`** (`SELECT … FOR UPDATE` on the
  match row). Without it, two simultaneous submits could each see the other as "not yet submitted" and
  neither would flip to judging (both submitted, match wedged in `drawing`). Keep the lock at the top of
  the submit tx.
- **A/B is positional and comes from SQL order.** `GetSubmissionsForJudging` orders by
  `(submitted_at, user_id)`: row 0 → image A, row 1 → image B. The judge sees only PNGs (never who is
  who); the game maps `A/B/tie` → player id / null (`GAME.md` §7.1). Change that `ORDER BY` and you
  silently re-bind the mapping.
- **Manual game e2e needs a clean match pool.** Open-pool matchmaking auto-joins the *oldest* waiting
  `open` match, so leftover open matches from a prior run hijack your new players (you end up dueling a
  ghost that never submits, and the match never reaches `judging`). Reset between manual runs:
  `delete from match_players; delete from drawings where match_id is not null; delete from matches;`
  (users/prompts/free drawings survive).
- **Logout is deliberately NOT behind `RequireAuth`** — it must clear the cookie even for an expired/
  anonymous caller. Don't wrap it in `protect()`.
- **Auth bodies use strict decode** (`DisallowUnknownFields`, 64 KiB); **document bodies use lax
  decode** (unknown fields tolerated, 8 MiB) for forward-compat. Don't unify them; the client
  `thumbnail` field is intentionally accepted-and-ignored.
- **`internal/document`, `internal/judge`, `internal/game`, and `internal/render` have Go tests** (the
  pure logic — validators, Elo, DTO redaction, stub coverage). auth/drawings/db/config/web/postgres and
  the DB-integration paths (`Submit`/`runJudging`/`persistResult`) have zero unit coverage — verified
  manually (curl/UI), per the ROADMAP. Grow tests where you touch.
- **`color.Color.RGBA()` returns alpha-PREMULTIPLIED 16-bit channels.** The judge's ink test
  (`internal/judge/fake.go`) shifts `>>8` to compare against an 8-bit threshold; since the judged
  raster is opaque, premultiplication is a no-op there. But any future ink/luminance heuristic on
  semi-transparent input must un-premultiply first, or partial-alpha color reads darker than it
  renders. `inkThreshold = 250` (not 255) deliberately absorbs anti-aliased stroke edges.
- **A detached background `go run` on Windows can be reaped without diagnostics** (observed exit code
  4, perfectly clean log). Symptom in the web app: save/load fails with the `network` `ApiError`. Just
  restart the server — there's nothing to debug in Go. (Internet scanners also hit a locally-exposed
  `:8080` — stray 404s like `/en/reviews` are noise.)
- **`curl localhost:8080` may hit a DIFFERENT app than our server.** Some other process on this machine
  binds `[::1]:8080` (IPv6 loopback specifically). Go's `:8080` listens on the `0.0.0.0` + `[::]`
  wildcards, but a specific `[::1]` bind wins for `localhost` when it resolves to `::1` first — so
  `curl http://localhost:8080/healthz` returned a stray Quasar HTML page, not our `{"status":"ok"}`.
  **Verify against `http://127.0.0.1:8080` (forces IPv4) to reach our server.** `netstat -ano | grep :8080`
  shows the competing PIDs.

## Konva / editor / render determinism

- **Konva keeps every `Stage` in a module-global registry until `stage.destroy()`.** Dropping the Vue
  ref alone leaks the stage and its `<canvas>` elements. `Editor.destroy()` exists for this — DrawView
  calls it in `onBeforeUnmount`, and every `loadDocument()` destroys+rebuilds the stage. Any new host
  of `Editor` must call `destroy()` on unmount.
- **Pointer coords come from `stage.getRelativePointerPosition()`** (transform-aware), never
  `pageX - offsetLeft` (the old DPR bug). Fit/zoom/pan (`packages/editor/src/view.ts` +
  `Editor.applyView`) scale the Konva **stage** (`size` + `scale` + `position`), never a CSS transform
  on the `<canvas>` — so `getRelativePointerPosition` keeps returning logical coords at any zoom.
  `applyView()` is re-run after every `rerender()` (which recreates the stage) or the transform resets.
- **Per-layer isolation is a render-contract requirement**: each document layer → its own
  `Konva.Layer` (own canvas) so composite strokes (eraser `destination-out`) can't bleed across
  layers.
- **`Editor.loadDocument()` does not validate** — callers pre-validate with `parseDocument()`
  (DrawView and `drawings.get` already do). Internal editor state is trusted.
- **Undo/redo covers the DOCUMENT, not editor view-state — by design.** Commands
  (`packages/editor/src/history.ts`) mutate only the `Document`, keyed by `id`; the **active layer**
  is editor UI state, not document state, so `setActiveLayer` is *not* undoable and add/remove only
  reassign the active layer as a side effect. Consequence: undoing (then redoing) a layer add/remove
  leaves the active-layer *selection* where the command left it, not where it was pre-command
  (`reconcileActiveLayer` only repairs a *dangling* active id). This is a conscious separation — keep
  editor UI state out of the pure command model; don't "fix" it by threading active-layer into
  commands. If `/play` ever needs full editor-state restore, model that as a separate concern.
- **`renderToPNG` is browser-only** (needs a real DOM + Konva stage) and is intentionally **not**
  unit-tested (no DOM in the Vitest runner). A headless/Konva-node server render path is a separate
  future thing (Phase 3 submit).
- **Konva 10 dropped its default Node.js backend** (browser is unaffected — the editor is fine). When
  the Phase-3 **headless server render worker** is built, it must `import 'konva/canvas-backend'`
  (or `konva/skia-backend`) before using Konva, or it won't render off-DOM. Also: Konva 10's one
  render-behavior change (rounded corners on **negative-dimension** rects) can't bite us — `rect.ts`
  normalizes to non-negative w/h and the validator rejects zero/negative dims, so v9↔v10 output is
  identical for every document we can produce.
- **`/draw` canvas fills its container; the document is `DEFAULT_CANVAS` (1920×1080)** and the editor
  fits it into the viewport (no more fixed 1280×720 + scroll). A `ResizeObserver` on the container
  auto-fits while `autoFit` is on; a manual zoom/pan turns `autoFit` off; `loadDocument` re-enables it.
  **Preview caveat:** `ResizeObserver` (like rAF/timers) is unreliable in the hidden preview tab, so a
  live `preview_resize` may not trigger a re-fit — verify a viewport-dependent fit by **reloading** at
  the target size (a fresh mount measures the container in the constructor), not by resizing live.
  Konva's `<canvas>` backing store is `stage.width × devicePixelRatio` (e.g. 375 CSS px → 750 px at
  DPR 2) — that's correct retina sizing, not a bug.

## Auth / cookies / dev proxy

- **Auth is cookie-session** (`jp_session`, HttpOnly) over same-origin `/api`. Dev relies on the vite
  proxy `'/api' → http://localhost:8080` (`vite.config.js`) to keep the cookie first-party; the Go
  server must be up on :8080 or every call throws `ApiError('network', 0)`.
- **`.env` sets `VITE_URL_API=/api`** (relative). Do **not** point it at an absolute cross-origin URL
  — the SameSite=Lax cookie stops being first-party.
- **`npm run preview` does not re-declare the proxy** (only the dev server does), so built output
  assumes a production reverse proxy for `/api` (an [IDEAS.md](IDEAS.md) item; every call 404s
  without it).
- The api layer is store-free by design — the session store calls the api, never the reverse (no
  import cycle).
- **`fetch` does NOT reject on 4xx/5xx** — only on network failure. The client turns `!res.ok` into a
  typed `ApiError` parsed from the `{error:{code,message}}` envelope, and uses a distinct `'network'`
  code (status 0) for a dead server. Keep both paths when extending it.

## Frontend / build

- **Cross-package `dist/` footgun:** `apps/web` consumes `@justpaint/document` / `@justpaint/editor`
  via their built `dist/` (`"*"` → `dist/index.js`). That `dist/` is **gitignored, not committed** —
  so a **fresh clone has no package `dist` at all**
  until you `npm run build` the packages, and `apps/web` / `vue-tsc` can't resolve
  `@justpaint/document|editor` before then. After a clone, and after editing package `src`, rebuild
  the package — there's no HMR across the boundary. (A Vite src alias is a deferred IDEAS.md item.)
- **`apps/web/tsconfig.json` does NOT extend `tsconfig.base.json`** — it re-declares its own strict
  flags and **omits** `noUncheckedIndexedAccess` / `noImplicitOverride` / `noFallthroughCasesInSwitch`.
  So the app is type-checked less strictly than the packages; a strict-only bug can pass
  `npm run types -w @justpaint/web` yet fail in a package.
- **noUncheckedIndexedAccess (packages):** `gesture[0]` is `T | undefined` — guard with an explicit
  `=== undefined` check, don't `!`-assert. Narrow union `Stroke` fields on `stroke.type` **before**
  touching them, in tests too — a runtime-green test can still fail `npm run types`. `satisfies Tool`
  keeps the frozen contract checked.
- **`URL.revokeObjectURL` in the same tick as `a.click()` can abort the download** in some browsers —
  defer the revoke (`setTimeout(…, 0)`).
- **oriui is a normal npm dependency** now (`@oriui/{vue,css,headless}` pinned to `1.0.0-alpha.2`, the
  owner's own alpha lib — was vendored under `vendor/oriui/` until the swap). The three move in
  **lockstep** — bump all three together (`@oriui/vue` pins its `css`/`headless` deps to the exact
  same version). `@oriui/css` must be imported for side effects (done in `main.ts`) or components
  render unstyled. A dep **version change needs a Vite dev-server restart** (Vite pre-bundles deps),
  not just an HMR reload.

## Preview MCP / verification

- **`preview_screenshot` times out (~30 s) on the Konva canvas page** — Konva's rAF loop likely
  defeats idle detection. Verify `/draw` with `preview_eval` (DOM / `getComputedStyle`),
  `preview_snapshot`, and console/network logs instead; don't retry screenshots there.
- The vite dev server runs via `.claude/launch.json` (`preview_start` name `web`, port 7777). Previewing
  the full app also needs the Go backend on :8080 up separately (not in launch.json).

## Orchestration / role agents

- Custom agents in `.claude/agents/*.md` load into the Agent/Workflow registry **at session start** —
  files created mid-session aren't available until Claude Code reloads. For a same-session workflow,
  omit `agentType` and inline the role instructions in the `agent()` prompt.
- The orchestrator (main session) integrates: review lenses write only findings; the orchestrator
  owns shared files (route wiring, barrels/exports, migration numbering), runs the gates, verifies
  live, and records findings here / in DECISIONS.md.
- **Adversarial verify passes have repeatedly caught real defects green unit tests missed**
  (type-level `noUncheckedIndexedAccess` violations, the Konva stage leak) — keep a verify stage in
  any orchestrated build.

## Git / Windows

- Conventional Commits, present tense, one logical change. Multi-commit units go on a branch
  integrated with `--no-ff` (then delete the branch); single-commit work goes straight to `main`.
  See [`CONTRIBUTING.md`](../CONTRIBUTING.md).
- `LF will be replaced by CRLF` warnings on commit are normal on Windows — harmless.
