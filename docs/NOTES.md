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
- **`internal/drawings` has a DB-backed integration test** (`roundtrip_test.go` `TestNameRoundtrip_DB`)
  that needs `DATABASE_URL` pointing at a **migrated** database — it `t.Skip`s otherwise. Plain
  `go test ./internal/drawings/...` silently skips the COALESCE default/keep-name SQL semantics unless
  the DB env is exported (docker compose up + goose up first).
- **`GetMatchPlayerDrawing` (the opponent-canvas reveal) binds its ids POSITIONALLY, and the roles are
  security-load-bearing.** `$1 = match_id`, `$2 = target_user_id` (whose drawing is returned), `$3 =
  viewer_user_id` (the authenticated caller, the membership gate). The `service.go` call site is
  field-named so it's safe, but if anyone reorders the generated `GetMatchPlayerDrawingParams` fields or
  swaps `$2`/`$3` in the SQL, viewer/target flip silently — the membership `exists` gate would key on the
  path-supplied target, not the JWT caller: an IDOR with **no compile error and no test failure**. Two
  guards exist: the `d.owner_id = $2 and d.match_id = $1` backstops (a flip fails them closed), and the
  field-named struct call. Still owed: a DB-integration regression test (skip-without-`DATABASE_URL`,
  like `roundtrip_test.go`) asserting a **non-member viewer gets 404 even when the named target has
  submitted** — that's the one assertion that would catch a role flip.
- **`users.rating` is written as an ABSOLUTE value (`UpdateUserRating`: `set rating = $2`), with no row
  lock and no CAS** — correct only because a single match resolves exactly once (the
  `GetMatchForUpdate` lock + status recheck that guards it, `service.go`/`deadline.go`). It is **NOT**
  safe against the same user's rating being updated by two *different* matches resolving concurrently
  (a lost update: both read the same pre-match rating, both write their own `after`, one clobbers the
  other) — and nothing stops a user from being seated in more than one active match today. Don't lean
  on `users.rating` as a precise ladder number under concurrent resolution without fixing this: either
  `rating = rating + delta` (atomic, no read-then-write) or serialize per user
  (`pg_advisory_xact_lock(hashtext(userID))`, the same idiom already deferred for the open-match dedupe
  gap above). Flagged, not fixed, by `feat/round-deadline` — forfeit/sweeper resolution is what first
  makes two-matches-at-once a realistic way to hit it (`IDEAS.md`).
- **Deadline enforcement leans on Postgres `now()` being the TRANSACTION-START instant, not the
  statement instant.** `GetMatchForUpdate`'s `server_now`, `StampSubmission`'s `now()`, and
  `SetMatchDrawing`'s deadline stamp are all plain `now()` — the SAME instant throughout one
  transaction, by Postgres semantics. This is exactly what makes the intended fair behavior work: a
  submit whose tx **began** before the deadline but whose row lock was only granted after it (queued
  behind the sweeper or another submit) still reads a `server_now` from *tx start*, sees itself as
  on-time, and stamps. **Do not "fix" these to `clock_timestamp()`** (the true wall clock, re-evaluated
  per call) — that would re-introduce exactly the race this design avoids, rejecting a submit that
  queued in time but was merely served late.
- **The sweeper's `List…` queries (`ListExpiredDrawingMatches` etc.) run on the pool in autocommit —
  their `FOR UPDATE SKIP LOCKED` lock only lasts for the SELECT itself**, released the instant it
  returns rows. The real exactly-once guard is the **per-row re-lock + status recheck** in
  `resolveExpiredMatch`/`refireJudging`/`reapOpenMatch` (each opens its own tx, re-`GetMatchForUpdate`s,
  and bails if the status already moved on) — the list is just a candidate scan, never trusted alone.
  Corollary: `RunSweeper`'s boot `drain()` loops each phase until it handles **fewer than a full batch**
  (rows HANDLED without error, not rows listed), so a batch that persistently fails returns a short
  count and drain exits rather than hot-looping a retry storm against an unhealthy DB — the 3s ticker
  still retries the tail on its normal cadence.
- **There is no `round_expired` error code.** A late submit (`ErrRoundExpired` in `internal/game`) maps
  to the existing generic `409 conflict` with `message: "round expired"` — the frozen v1 error-code set
  (`API.md` §3) was deliberately NOT extended for this. The client keys off "any submit `409` → go poll
  the result," not a distinct machine code, so there was nothing for a new code to buy.

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
- **Canvas-bounds & preview compositing (feat/draw-ux, DECISIONS 2026-07-04):** every projected
  layer is **clipped to the document rect** (`toLayer`/`backgroundLayer`, `clip:{0,0,w,h}` in
  layer-local coords — follows any stage/layer transform, so it holds in the editor at any zoom AND
  in the render worker); gestures **starting outside** the document are ignored
  (`Editor.insideDocument`); the in-flight stroke preview renders in a transient `Konva.Group` **on
  the active layer's own Konva layer** — never an isolated preview layer, which could not show the
  eraser's `destination-out` erasing content beneath it (the old delayed-eraser bug); window-level
  `pointerup`/`pointercancel` fallbacks end/abandon a gesture released outside the container.

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
- **oriui is a normal npm dependency** now (`@oriui/{vue,css,headless}` pinned to `1.0.0-alpha.10`, the
  owner's own alpha lib — was vendored under `vendor/oriui/` until the swap). The three move in
  **lockstep** — bump all three together (`@oriui/vue` pins its `css`/`headless` deps to the exact
  same version). `@oriui/css` must be imported for side effects (done in `main.ts`) or components
  render unstyled. A dep **version change needs a Vite dev-server restart** (Vite pre-bundles deps),
  not just an HMR reload.
- **`--ori-color-outline` is justpaint-invented (oriui ships NO outline token):** the resolved alias
  must be set at base `:root` for light AND repointed in the dark block, or light-mode borders
  silently fall to the black literal fallback.
- **`OriButton size="sm"` does not shrink height below ~40px** (`size` only sets `--ori-size-action`;
  height is `max(2.5em, action)`) — budget floating-cluster widths accordingly.
- **`layersOpen` is computed once at mount** from `innerWidth` and not re-evaluated on resize;
  `color-mix()` (first used in the shell) sets the browser floor at ~2023 evergreens;
  `eslint-disable-next-line` in SFC templates covers only the literal next LINE — with
  attribute-per-line formatting it must sit right above the `v-html=` line (or use a block disable).
- **`OriDialog` is CONTROLLED as of alpha-11 — RESOLVED 2026-07-10.** Through alpha-10 it had no `open`
  prop or close emit (only `defaultOpen`/`closeOnEscape`/`closeOnInteractOutside`/`modal`/`title` + a
  `trigger` slot), so external state (a hotkey, a chip toggle) couldn't drive it and ConfirmDialog /
  ShortcutsDialog were hand-rolled controlled modals (Teleport + `Transition :duration` +
  `tabindex="-1"` focus-on-open + panel-tree Esc + backdrop click) — that pattern is now historical.
  Alpha-11 added `open` + `update:open`/`close` emits (`v-model:open`); both are migrated to
  `OriDialog`. `OriKbd` still ships as the kbd-chip style — use it over custom chips.
- **An UNLAYERED universal reset silently clobbers oriui's LAYERED box-model** (`feat/draw-ux-polish`,
  2026-07-05). Cascade rule: unlayered author CSS beats **all** `@layer` styles regardless of
  specificity. oriui ships `.ori-button { border: 1px solid …; padding-inline: 1em }` and
  `.ori-input__field { border: 1px solid … }` inside `@layer ori.components`, so an unlayered
  `* { border: 0; margin: 0; padding: 0 }` in `apps/web/src/reset.css` overrode them — every
  `OriButton(outline)` rendered borderless and every `OriButton`/`OriInput` got `padding-inline: 0`
  (the top-right Save button collapsed to 33px wide / 0 padding, reading as a cramped text-link).
  oriui's OWN `@layer ori.reset` already does `*{margin:0;padding:0;border:0}` **layer-safely** (it
  loses to `ori.components`, as intended), so the app must NOT re-declare box-model resets unlayered —
  delegate the reset to oriui and add only app-specific bits (font-optical-sizing, canvas
  `touch-action`, etc.). Fix: dropped `border`/`margin`/`padding` from the `*` rule in `reset.css`.
  apps/web now carries NO `.ori-*` overrides (the last one — the light colored-text tone — went with
  oriui alpha-8's role-as-text AA tokens); no rule targets `.ori-*`, no `:deep()`, no `!important`.
- **The one sanctioned `.ori-*` override: light-theme colored-TEXT tone.** **RESOLVED alpha-8 (2026-07-08)** — kept below as history. Outline/tonal/text OriButtons
  read their label color from `--ori-color`; the brand `hsl(20 100% 50%)` is only 2.86:1 on the
  `#f0f2f6` surface (fails AA text). `main.css` darkens the TEXT tone to `hsl(20 100% 38%)` for
  `.ori-button.ori-color_primary:where(.ori-variant_outline, _tonal, _text)` in light only — fills keep
  the vivid brand (contrast there is the dark ink ON orange). NB: every OriButton always carries
  `ori-color_<role>`, so guard by pinning `ori-color_primary`, NOT by `:not([class*='ori-color_'])`
  (that matches everything and disables the rule).
- **OriPopover/OriMenu position via CSS Anchor Positioning — not in Firefox** (mid-2026). The popover
  still opens/dismisses (native Popover API is baseline) but ignores `position-anchor`/`position-area`,
  falling back to the UA default (viewport-centered) — degraded but functional. Fine for the toolbar's
  mobile style panel; don't build anchor-critical UI on it yet.
- **Don't center overlays with `position:absolute; left:50%; translateX(-50%)`.** An abs-positioned box
  with a `left` offset shrink-to-fits against the REMAINING space (viewport − left), so on a 405px phone
  the bottom toolbar got squeezed to min-content and wrapped. Use a full-width strip instead:
  `left:0; right:0; display:flex; justify-content:center` + `pointer-events:none` on the strip and
  `pointer-events:auto` on the child.
- **The registry `@oriui/css` alpha-6 `OriTooltip` mis-pairs the bubble** — background comes from
  `--ori-color`, but text from `--ori-color-on` (= the global `currentColor`), so on dark ink the
  tooltip renders **black-on-black**. App-side interim fix pins `--ori-tooltip-bg`/`--ori-tooltip-color`
  on `.ori-tooltip`. The **root** fix is in oriui (self-pairing bubble + viewport flip/clamp + a static
  arrow — the `anchor(center)` arrow was invalid CSS), on `feat/neutral-skin-tooltip-contrast`
  (commit `bd3d847`, unpublished). **RESOLVED alpha-7 (2026-07-08):** the app bumped and the interim
  override was dropped; the published bubble is self-paired + anchored.
- **Preview-testing an UNPUBLISHED oriui build rewrites `package.json`.** `npm install <abs path>.tgz`
  on a local oriui tarball rewrites the `@oriui/*` deps to fragile `file:` refs pointing at the sibling
  `vueinjar` repo — do **NOT** commit those. Revert `package.json` + the lockfile to the registry range
  and keep the built `node_modules` local for the duration of the preview (precedent: the alpha-4
  preview tarballs).
- **Stale Vite dep-optimize cache after an out-of-band SAME-VERSION dep swap.** A long-running `vite
  dev` pre-optimizes deps into `node_modules/.vite` (and `apps/web/node_modules/.vite`) at startup. If
  node_modules later changes out-of-band but the dependency **keeps the same version string** — e.g.
  swapping a local `@oriui/* 1.0.0-alpha.6` tarball for the registry build of the same
  `1.0.0-alpha.6`, or vice versa — a **reused** dev server (Playwright `webServer.reuseExistingServer:
  true`, or just an already-running `npm run dev`) keeps serving the **stale** optimized bundle, mixing
  an OLD bundled component with the app's CURRENT CSS. **Symptom this cost us (2026-07-08):** the
  `/draw` **mobile** layout looked badly broken — the stale bundle's OriTooltip emitted the old
  `.ori-tooltip__bubble_bottom` class (which the current `@oriui/css` no longer styles), so each hidden
  tooltip bubble fell back to `position: static` (in normal flow), adding ~40px width to every wrapped
  control; the top actions island ballooned to ~194px tall and the bottom toolbar's Tools group
  overflowed (~624px inside a ~350px bar). The **locally-installed oriui (the branch tarball) and a
  clean `vite build` off it were correct the whole time** — that tarball ships the anchored bubble
  (`.ori-anchored { position: fixed }`, out of flow) and the dedicated `--ori-neutral-900/50` pairing;
  the break was purely the stale local dev bundle masking it. **NB — do not conflate this with the
  registry:** PUBLISHED `@oriui/css` alpha-6 WAS the OLD in-flow / mis-paired tooltip (the fix was
  on the oriui branch `bd3d847`, since published in alpha-7 — the app now runs alpha-10)
  (the published alpha-7+ bubble is anchored + self-paired, so no app override is needed — the
  stale-cache hazard is purely about mismatched bundles at the same version string). **Fix (the stale
  cache):** `rm -rf node_modules/.vite apps/web/node_modules/.vite`, then
  restart the dev server (a fresh start re-optimizes from current node_modules — verify the OriTooltip
  bubble computes `position: fixed` with classes `ori-anchored ori-anchored_<placement>`). **Guard:**
  after ANY out-of-band oriui tarball↔registry swap at the SAME version string, clear `.vite` and
  restart before trusting the preview. Cross-ref the tarball-swap note above and the parallel-oriui-dev
  hazard (HEAD can switch under feature work).
- **Two oriui CSS gotchas the `FloatingToolbar` → `OriToolbar` migration surfaced — both FIXED in oriui
  alpha-13 (2026-07-10); kept as the lesson (full detail in `DESIGN-SYSTEM.md` §6).**
  **(A) layer order beats specificity:** alpha-12's pressed *fill*
  (`.ori-toolbar .ori-button[aria-pressed=true]`, layer `ori.components`) was defeated by `.ori-variant_text`
  re-setting `--ori-variant-bg-color: transparent` in the LATER layer `ori.utilities`, so the active tool
  showed the inset ring only (no fill). *alpha-13:* the pressed rule paints `background-color` directly, not
  via the token. **(B) relative-colour transitions get stuck:** alpha-12's `.ori-button`
  `transition: color` couldn't interpolate oriui's `oklch(from … )` role tokens, so swapping `color` per
  selection left the glyph frozen on the previous colour until a repaint. *alpha-13:* `color` was dropped from
  the transition, so the swap is instant — the active tool now uses `:color="active ? 'primary' : 'surface'"`.
  **Corollary still live (it nearly tripped the review):** the focus ring stays visible on
  `color="surface"` buttons ONLY because `main.css`'s **unlayered** `:where(button, …):focus-visible {
  outline: var(--ori-color-primary) }` outranks oriui's layered `.ori-button:focus-visible { outline:
  var(--ori-color) }` (which for `surface` resolves to background-on-background = invisible). Unlayered
  author styles beat ANY `@layer` regardless of specificity — the same mechanic as (A), used in our favour.

## /play — async-duel client

- **The HTTP `fetch` plumbing is shared, not per-client.** `core/api/http.ts` owns `BASE`, the
  `ApiError` envelope, and `request`; `drawings.ts` **and** `matches.ts` import it. The barrel
  (`core/api/index.ts`) re-exports all three, so every `@core` consumer of `ApiError`/`isAuthError`
  is unaffected by the split — but a **new** api module must import `request` from `./http`, never
  re-implement the cookie/`credentials:'include'` + error-envelope logic.
- **`matches.ts` wire types are 1:1 with the Go DTOs** in `server/internal/game` (handler.go), NOT
  just with API.md — read the structs when changing them (e.g. `MatchResult` is a union discriminated
  on `ready`; scores are **0..1** from the judge and the reveal multiplies to 0..100; `judgedImageUrl`
  is always `null` until the object-storage seam lands).
- **PlayView runs exactly ONE self-rescheduling poll loop** for the whole round (waiting → drawing →
  judging), reached from `startMatch`. `submit()` does **not** start a second loop — it only flips the
  phase to `judging`; the existing loop (which reschedules through the transient `submitting` phase)
  picks up the verdict branch. Adding a `scheduleNextPoll()` in `submit` would double every poll.
- **The round timer is a soft client-side UX pressure only** — v1 has no server-authoritative
  deadline, so the countdown is not reconciled against the match (it auto-submits on expiry). Don't
  treat `remaining` as authoritative; a real deadline is a `TODO(play-api)`.
- **Every async continuation checks the `disposed` flag** (set in `onBeforeUnmount`) before touching
  reactive state — the poll loop + `await`ed create/submit/capture can resolve after the route
  changes. `clearTimers()` clears both the countdown interval and the tracked poll `setTimeout`s.
- **A duel is auth-required**; `/play` has no sign-in form. An anonymous visitor (or a `fetchMe`
  failure) routes to the sign-in card, which links to `/draw` (where the SideMenu auth lives). A `409`
  on submit is treated as already-recorded and proceeds to the verdict poll, not an error.

## WS realtime (`internal/ws`, `feat/ws-realtime`)

- **No server-side read-idle timeout/heartbeat.** The app-level `{"type":"ping"}` is **client→server
  only** — `readPump` answers it with a `pong` but nothing on the server side pings the client or
  arms a read deadline. A black-hole TCP drop (no FIN/RST, e.g. a yanked cable or a dead NAT mapping)
  is therefore only reclaimed by the next outbound write hitting `wsWriteTimeout` (10s, `conn.go`) or
  by the session-expiry close (≤7-day `sessionTTL`, `auth/token.go`) — whichever comes first — bounded
  in the meantime by the per-user-per-match connection cap (`wsMaxConnsPerUser = 5`, `hub.go`). Fine at
  this scale; a real idle-eviction deadline is an `IDEAS.md` item.
- **`MatchStateJSON`/`ResultJSON` → `Get`/`Result` read without a snapshot** — the same torn-read
  caveat already on record above (`Get` runs its match→prompt→roster reads on the pool with no
  snapshot). WS calls this path far more often than the 2s REST poll (on every roster/deadline change
  and again per reconnect), so it widens the same window rather than introducing a new one. Covered
  today by the client's monotonic/idempotent apply (`applyRoster`/`applyResult` in `PlayView.vue`,
  `docs/DESIGN-PHASE3-LIVE.md` §2.9) — a stale/torn read is just superseded by the next frame or poll.
- **Per-viewer `match_state`/`result` frames fan out on independent goroutines** (`fanoutPerViewer` is
  spawned per event, `hub.go`), so **completion order across the two duelists — or across two frames
  to the same duelist — is not guaranteed on the wire.** Correctness relies entirely on the client
  applying frames monotonically (a terminal phase can't regress), never on send order.
- **`websocket.Accept` reaches `http.Hijacker` by walking the `Unwrap() http.ResponseWriter` chain.**
  Any future `http.ResponseWriter` wrapper added to the middleware chain (logging, metrics, etc.) MUST
  implement `Unwrap() http.ResponseWriter` or the WS handshake fails with a `501` the moment that
  wrapper sits in front of the WS route — this is exactly why `statusRecorder.Unwrap`
  (`internal/platform/web/middleware.go`) exists. Don't drop it when touching `LogRequests`/`Recover`.
- **The `firstForUser` presence broadcast includes the connecting client itself** — when a user's
  client set goes empty→non-empty, `handleRegister` broadcasts `opponent_connected` to the **whole
  room**, including the socket that just triggered it. Harmless by design: clients filter presence
  frames by `userId !== self` (`PlayView.vue`'s `handleWsFrame`), so a client silently receives (and
  ignores) its own connect event rather than the hub special-casing the sender.
- **`parseToken` now requires an `exp` claim** (`jwt.WithExpirationRequired`, `auth/token.go`) — a
  valid token always carries an expiry, which is what makes the WS session-expiry close (arm
  `time.AfterFunc(exp)` → close `4001`) safe to rely on unconditionally. Any future token-issuing path
  must keep stamping `exp` or `RequireAuth` rejects it outright.

## AI assist (`internal/assist`, `packages/editor` ghost preview, feat/assist-phase-a)

- **The op validator does NOT presence-guard the inner stroke fields of `add_stroke`** — safe today
  ONLY because `freehand` (the one stroke type whose all-zero `brush` still passes the existing
  stroke validator) is excluded from the Op schema (`docs/ASSIST.md` §2). If Phase B/C ever admits
  freehand ops, or any future stroke type where a zero-valued field is legitimately valid, extend the
  `requiredOpKeys`-style presence guard (mirroring `parse.go`'s `requiredKeys`) in lockstep on both
  sides — otherwise Go silently zero-fills an absent field the TS side would reject.
- **The assist ghost overlay must remount inside `Editor.rerender()`**, following the same discipline
  the cursor overlay already uses — `rerender()` destroys and rebuilds the whole Konva stage — or the
  ghost silently vanishes on the next commit/undo/redo/`loadDocument`.
- **`addLayerCommand`'s index clamps at *apply* time, not construction.** `acceptOps()` threads a
  running top-of-stack index through a multi-`add_layer` batch (incremented per `add_layer` processed
  in array order), or every new layer in the batch would clamp to the same insertion point and
  collide in z-order.
- **`Retry-After` must be set BEFORE calling `web.Error`.** `web.Error` → `JSON` → `w.WriteHeader`,
  and headers set after `WriteHeader` are silently dropped by `net/http` — the assist handler sets it
  first on the 429 path (`server/internal/assist/handler.go`).

## Preview MCP / verification

- **`preview_screenshot` times out (~30 s) on the Konva canvas page** — Konva's rAF loop likely
  defeats idle detection. Verify `/draw` with `preview_eval` (DOM / `getComputedStyle`),
  `preview_snapshot`, and console/network logs instead; don't retry screenshots there.
- **`/play` needs the Go backend + a SECOND authenticated player** to reach `judging`/`done` — not
  drivable from one browser preview. Smoke-test the mount + auth-gated card without a backend
  (`/api/auth/me` 502 → the sign-in card renders); the client shapes are verified against the Go DTOs
  by reading them, not by driving the full happy-path.
- The vite dev server runs via `.claude/launch.json` (`preview_start` name `web`, port 7777). Previewing
  the full app also needs the Go backend on :8080 up separately (not in launch.json).
- **rAF is fully PAUSED while the preview tab is hidden** (`document.hidden === true`) — Konva's
  `batchDraw` never flushes, so layer canvases keep stale pre-transform content; pixel-level canvas
  checks are IMPOSSIBLE in the hidden preview (this also explains the screenshot timeout above).
  Verify canvas pixels via the **Node render worker** instead (same Konva projection, deterministic
  — render a crafted document and sample the PNG), or a real visible tab; DOM-level checks (stroke
  counters, panel state) still work hidden. Vue `<Transition>` completion also hangs in the hidden
  preview tab (its pipeline uses rAF), so menu/panel open-close animations can't be end-verified
  there — check them in a real browser.
- **CSS transitions also freeze mid-flight in the hidden tab** — and that includes property values:
  a button with `transition: color …` reports the OLD `getComputedStyle(...).color` indefinitely even
  though its `--ori-*` custom properties (not transitioned) already show the new value. If a computed
  color "didn't change" but the tokens did, reload the page (fresh paint has no transition) before
  concluding the CSS is broken.
- **The desktop cursor-coord readout is rAF-throttled, and rAF is PAUSED in a hidden preview tab** — so
  the coords chip never appears under `preview_eval` there, and any eval that `await`s
  `requestAnimationFrame` **hangs to the 30 s timeout**. Verify the coord readout in a real visible
  browser; never `await` rAF in a preview eval.

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

## Document contract (Go ↔ TS parity)

- **Absent required fields: Go must reject them explicitly.** `encoding/json` zero-fills an absent
  required key (no `visible` → `false`, no `opacity` → `0.0`, no `brush` → the zero `BrushOptions`, no
  `strokes` → nil, no `background` → nil), so struct decoding ALONE makes Go accept documents the TS
  validator rejects — a silent keystone-parity break. `requiredKeys(data)` in `parse.go` walks the RAW
  JSON asserting key presence, so an explicit `null` background still counts as present (valid) while an
  absent one is rejected — matching TS, where `undefined !== null` falls into the hex check and fails.
- **Point coords: decode `[]*float64`, not `[]float64`.** `json.Unmarshal` coerces a `null` array
  element to `0.0`, so `[null,1]` would pass; a pointer slot stays nil and is rejected, matching TS's
  finite-number check.
- **The two validator TEST TABLES are 1:1** — a rejection added to one side must be added to the other
  (`server/internal/document/validate_test.go` ↔ `packages/document/test/validate.test.ts`).

## Git / Windows

- Conventional Commits, present tense, one logical change. Multi-commit units go on a branch
  integrated with `--no-ff` (then delete the branch); single-commit work goes straight to `main`.
  See [`CONTRIBUTING.md`](../CONTRIBUTING.md).
- `LF will be replaced by CRLF` warnings on commit are normal on Windows — harmless.
