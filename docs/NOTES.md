# Implementation notes

Non-obvious gotchas: traps in this codebase that are easy to step into and hard to see from one file.
Read the section for an area before changing it, and add a note when you find a new trap. Rationale
lives in [DECISIONS.md](DECISIONS.md), the per-change bar in [REVIEW.md](REVIEW.md).

## Document contract

### The server validator is the only one

`server/internal/document` is the only check of the format; the TS types in
`packages/editor/src/document` only describe it. Change the spec, the Go validator and the TS types
together, then refresh the fixture (`npx vitest run -u` in `packages/editor`) so `TestEditorDocument`
confirms the server still accepts what the editor draws.

### Go accepts absent required fields unless you check key presence

`encoding/json` zero-fills a missing key (no `visible` → `false`, no `opacity` → `0`, no `brush` → the
zero `BrushOptions`, no `background` → nil), so struct decoding alone accepts documents the spec rejects.
`requiredKeys` (`parse.go`) and `requiredOpKeys` (`ops.go`) check presence on the raw JSON, which also
keeps an explicit `null` background valid while an absent one fails. A new required field needs a
presence check there too.

### Point arity and `null` coordinates need custom decoding

`encoding/json` zero-fills short fixed-size arrays, drops extra elements and turns a `null` element into
`0`. `FreehandPoint` (3) and `Point` (2) therefore have a custom `UnmarshalJSON` that decodes into
`[]*float64` and checks length and nil. Change those types without keeping the guard and `[1]` or
`[null,1]` gets through.

### Colors are lowercase hex only

The validator uses `^#([0-9a-f]{6}|[0-9a-f]{8})$`; `#FFFFFF` is a 400. Nothing normalizes case, so
any code that produces a color (a picker, AI output) must lowercase it.

### Adding a stroke type touches three Go sites plus the TS types

`Stroke` is a sealed interface (unexported `base()`), so it cannot be extended from outside the
package. A new type needs the struct and const in `document.go`, the switch in `unmarshalStroke`
(`parse.go`), the switch in `checkStroke` (`validate.go`), and the TS types.

### `FREEHAND_VERSION` must equal the installed perfect-freehand

`packages/editor/src/document/constants.ts` pins the exact resolved version (`1.2.3`, from `^1.2.3` in
`packages/editor`), not the range floor. Bump it with the dependency, or the editor preview and the
render worker (`packages/render`) draw different outlines — and the worker's raster is what gets judged.

### Round only on the way to the server

Write precision (2 dp geometry, 3 dp pressure) is applied by `roundDocument`
(`packages/editor/src/document/round.ts`) in the API clients, on every document the app sends.
Rounding the live model or in tools accumulates error.

### Ids share one namespace

Layer and stroke ids live in one `seen` set; a stroke id equal to a layer id is rejected.

### The op validator does not presence-check `add_stroke`'s inner stroke

`requiredOpKeys` checks `layerId` and `stroke`, not the fields inside the stroke. That is safe only
because `freehand` (whose all-zero `brush` passes the stroke validator) is not allowed in ops
(`docs/ASSIST.md` §2). If ops ever admit freehand, or a stroke type where a zero value is valid, extend
the presence guard in `ops.go`.

## Editor and rendering

### Destroy the `Editor` on unmount

Konva keeps every `Stage` in a module-global registry until `stage.destroy()`, so dropping the Vue ref
leaks the stage and its canvases. Every host must call `editor.destroy()` in `onBeforeUnmount`
(DrawView, PlayView and PracticeView do).

### Pointer coordinates come from the stage transform

Read positions with `stage.getRelativePointerPosition()`, never `pageX - offsetLeft`. Fit/zoom/pan
(`view.ts`, `Editor.applyView`) transform the Konva stage, never the `<canvas>` via CSS, so logical
coordinates hold at any zoom. `rerender()` recreates the stage, so `applyView()` must run after it or
the transform resets.

### Overlays must remount inside `rerender()`

`rerender()` destroys and rebuilds the whole stage on every commit, undo/redo and `loadDocument`. Chrome
that is not in the document — the brush cursor ring, the AI-assist ghost preview — must be remounted
there or it silently disappears.

### Layer isolation, clipping and the stroke preview

Each document layer is its own `Konva.Layer`, so composite strokes (eraser `destination-out`) cannot
bleed across layers. Every projected layer is clipped to the document rect in layer-local coordinates,
which holds in the editor at any zoom and in the render worker. The in-flight stroke preview renders in
a `Konva.Group` on the active layer's own Konva layer; a separate preview layer could not show the
eraser removing what is beneath it. Gestures must start inside the document (`insideDocument`), and
window-level `pointerup`/`pointercancel` listeners end a gesture released outside the container.

### `Editor.loadDocument()` does not validate

It trusts its input: documents come from the server, which validated them on write, or from the
app's own blank-document builders.

### Undo covers the document, not editor state

Commands (`history.ts`) change only the `Document`. The active layer is UI state: `setActiveLayer` is
not undoable, and undoing a layer add/remove leaves the selection where the command left it
(`reconcileActiveLayer` only repairs a dangling id). Intentional; don't thread UI state into commands.

### Multi-layer assist batches need a running insert index

`addLayerCommand` clamps its index at apply time, not at construction. `acceptOps()` threads a running
top-of-stack index through the batch; without it every new layer in a batch lands at the same position.

### Auto-fit turns off on manual zoom

The document (`DEFAULT_CANVAS`, 1920×1080) is fitted to its container by a `ResizeObserver` while
`autoFit` is on. A manual zoom or pan turns it off; `loadDocument` and "fit" turn it back on. The canvas
backing store is `stage.width × devicePixelRatio` — correct retina sizing, not a bug.

### `renderToStage` is the one projection

The browser export (`renderToPNG`, `stage.toBlob`) and the Node worker (`stage.toDataURL()` under
`konva/canvas-backend`) both go through `renderToStage`. Keep render changes on that path or the judged
raster drifts from what the player saw. `renderToPNG` needs a real DOM and is not unit-tested; the
worker has its own `selftest`.

### The render worker: backend import first, bundled, rebuilt by hand

- `import 'konva/canvas-backend'` must be the first import in `packages/render/render.mjs`. Konva 10
  has no default Node backend and throws "unsupported environment" without it. `document` is not
  polyfilled, which is why `toKonva` guards on `typeof document`.
- The worker is an esbuild bundle (`packages/render/dist/render.mjs`) because the editor is
  TypeScript with extensionless imports, which native Node ESM refuses. The bundle embeds a copy of
  the editor: rebuild it (`npm run build -w @justpaint/render`) after editing `packages/editor`, and on a fresh
  clone before using `RENDER_MODE=node`.
- `canvas` (node-canvas) is a native dependency, kept external. It normally installs from a prebuild; a
  platform without one needs Cairo/Pango and build tools.

### `StubRenderer` does not draw the document

`RENDER_MODE=stub` (the default, so the server runs without Node) emits a deterministic 1024² PNG whose
ink coverage grows with the stroke count, which is enough for the ink-coverage `FakeJudge` to produce a
document-derived verdict. It is not the picture. The real raster is `RENDER_MODE=node`; don't build a Go
rasterizer instead, it would diverge from the editor.

### Node render concurrency is bounded inside the renderer

`NodeRenderer` spawns `node dist/render.mjs` per render (document JSON on stdin, base64 PNG on stdout,
which keeps the pipe text-safe on Windows). A semaphore inside the renderer, sized by
`JUDGE_CONCURRENCY`, bounds concurrent processes, because `/api/guess` and `/api/practice` render on the
request goroutine and bypass the judging-pass limiter. It blocks rather than refuses; the wait ends with
the caller's context (`game.JudgePassBudget`, the practice/guess `RunBudget`).

## Frontend

### The editor is consumed from source

`@justpaint/editor` exports `src/index.ts`: Vite, vue-tsc and the render worker's esbuild compile it
directly, so nothing needs building before a typecheck and edits hot-reload into the app.

### `apps/web` is type-checked less strictly than the packages

`apps/web/tsconfig.json` does not extend `tsconfig.base.json` and omits `noUncheckedIndexedAccess`,
`noImplicitOverride` and `noFallthroughCasesInSwitch`, so code can pass
`npm run types -w @justpaint/web` and fail in a package. In packages `arr[0]` is `T | undefined`: guard
with `=== undefined` rather than `!`, and narrow a `Stroke` on `type` before touching its fields —
tests included.

### Keep the API same-origin

The `jp_session` cookie (SameSite=Lax), the WS upgrade and reading `Retry-After` all depend on it.
The request base is the constant `/api` in `http.ts`. In dev the Vite proxy forwards it (with `ws: true`)
to `:8080`, so the Go server must be up or every call fails with `ApiError('network', 0)`. In production
the Go server serves the built SPA itself (`STATIC_DIR`). `npm run preview` has no proxy, so `/api`
fails there.

### Build new API modules on `core/api/http.ts`

`http.ts` owns `BASE`, `ApiError` and `request` (`credentials: 'include'`, envelope parsing). `fetch`
rejects only on a network failure; `request` turns `!res.ok` into a typed `ApiError` from the
`{error:{code,message}}` envelope and uses the client-only `network` code (status 0) for a dead server.
A new module imports `request` instead of re-implementing it. The api layer is store-free: stores call
it, never the reverse.

### Defer `URL.revokeObjectURL` after a download click

Revoking in the same tick as `a.click()` can abort the download in some browsers; use
`setTimeout(…, 0)`.

### Don't center overlays with `left: 50%; translateX(-50%)`

An absolutely positioned box with a `left` offset shrinks to fit the remaining width, so on a narrow
phone the toolbar wrapped. Use a full-width strip (`left: 0; right: 0; display: flex;
justify-content: center`) with `pointer-events: none`, and `pointer-events: auto` on the child.

### Small ones

- `layersOpen` (DrawView) is computed once at mount from `innerWidth`, not re-evaluated on resize.
- In SFC templates `eslint-disable-next-line` covers only the literal next line; with one attribute per
  line it must sit directly above the offending attribute, or use a block disable.

## /play — async-duel client

### Match wire types mirror the Go DTOs

`core/api/matches.ts` follows the structs in `server/internal/game/handler.go`, not only `API.md`.
`MatchResult` is a union on `ready`; judge scores are 0..1 and the reveal shows 0..100;
`judgedImageUrl` is always `null` (there is no object storage — the reveal uses the participant-drawing
endpoint).

### `/play` runs exactly one poll loop

`PlayView` starts one self-rescheduling poll loop in `startMatch` for the whole round. `submit()` only
flips the phase to `judging`; calling `scheduleNextPoll()` there would double every poll. A live socket
slows the loop down but never stops it.

### The round countdown is anchored on the server clock

`anchorClock(drawingDeadline, serverTime)` stores the offset between server and browser clocks from
every roster/create/submit response, so both duelists count down from the same server instant and a
skewed or suspended client cannot gain time. The countdown only mirrors the deadline: Postgres is the
authority, a late submit gets `409` whatever the client shows, and the client treats any submit `409`
as "go poll the result".

### Check `disposed` after every `await`

The poll loop and the awaited create/submit/capture calls can resolve after the route changes. Every
continuation checks `disposed` (set in `onBeforeUnmount`) before touching state, and `clearTimers()`
clears the countdown and the tracked poll timeouts.

### A finished duel patches the session rating directly

`applyResult` writes `session.user.rating`, because the session store sets it only on
restore/login/register. A store refactor must keep this sync (or replace it with a store action), or
every other rating display is stale after a duel.

## oriui and CSS

### Unlayered CSS beats every `@layer`

oriui's styles live in cascade layers, and any unlayered app rule wins regardless of specificity. An
unlayered `* { border: 0; margin: 0; padding: 0 }` in `reset.css` once stripped every `OriButton`'s
border and padding. oriui already resets the box model inside `@layer ori.reset`, so the app must not
re-declare it unlayered. The same mechanism is used on purpose: the focus ring on `color="surface"`
buttons is visible only because `main.css`'s unlayered `:focus-visible` outline beats oriui's layered
one, which resolves to an invisible color there.

### The oriui CSS import list is hand-maintained

`main.ts` imports `@oriui/css/components/*.css`, one file per component. A missing line renders that
component unstyled with no error. `npm run lint:styles` (`apps/web/scripts/check-styles.mjs`, part of
`lint:all` and `lint:ci`) guards it by checking selectors, not filenames, because some component CSS is
inlined elsewhere (`.ori-spinner` ships in `button.css`).

### oriui packages move in lockstep

`@oriui/vue`, `@oriui/css` and `@oriui/headless` are pinned to one exact version (`1.0.0-rc.18`), and
`@oriui/vue` pins the other two to its own, so bump all three together. `@oriui/css` must be imported
for its side effects or components render unstyled.

### After a dependency change, clear Vite's pre-bundle

Vite pre-bundles dependencies into `node_modules/.vite` and `apps/web/node_modules/.vite` at startup. A
running dev server keeps serving the old bundle after an install, and a restart does not help when the
version string is unchanged (e.g. swapping a local oriui tarball for the registry build of the same
version). The tell: a new prop falls through to the DOM as a raw attribute (`pressed="true"` and no
`aria-pressed`), because the component actually rendered is not the one in `dist`. Stop the dev server,
`rm -rf node_modules/.vite apps/web/node_modules/.vite`, restart, then judge.

### Installing a local oriui tarball rewrites `package.json`

`npm install <path>.tgz` rewrites the `@oriui/*` dependencies to `file:` references. Never commit them:
revert `package.json` and the lockfile to the registry version when the local test is done, and clear
the Vite cache (above).

### `--jp-color-outline` is ours, not `--ori-color-outline`

The hairline token must be set at `:root` for light and repointed in the dark block, or light borders
fall back to black. oriui now ships `--ori-color-outline`, but it is a `currentcolor` tint; ours is a
fixed per-theme color held to the 3:1 non-text bar by `scripts/check-contrast.mjs` (`DECISIONS.md`
2026-09-18).
Same name, different job; don't switch components to the oriui one.

### A `<dialog>` inside the shell overlay is unclickable

`EditorShell`'s `.shell__overlay` is `pointer-events: none` (each island opts back in). A native
`<dialog>` renders in the top layer but still inherits `pointer-events` from its DOM ancestors, so its
buttons and backdrop are dead to the mouse while Escape keeps working. `.shell__overlay :deep(dialog)
{ pointer-events: auto }` fixes dialogs inside the shell (`:deep()` because the dialog belongs to a child
component); app-wide dialogs such as the sign-in modal live in `App.vue`, outside any such ancestor.
Diagnose with `document.elementFromPoint()` on the button: it returns `<html>`.

### A disabled `OriButton` cannot say why

`.ori-button:disabled` sets `pointer-events: none` and a disabled button takes no focus, so its tooltip
never opens — on a phone a disabled icon button is an unexplained dim glyph. When the reason is not
self-evident, keep the button enabled and explain in the result (the AI-guess trigger does this). Undo
and redo on a fresh canvas need no explanation.

### `OriButton size="sm"` stays about 40px tall

`size` sets `--ori-size-action`, but the height is `max(2.5em, var(--ori-size-action))`. Budget floating
clusters accordingly.

### `OriPopover`/`OriMenu` rely on CSS anchor positioning

In a browser without it (Firefox, as of mid-2026) the popover still opens but sits at the UA default
position. Check current support before building anchor-critical UI on it.

## Go backend

### Setup

- The server does not load `.env`. `ENV`, `DATABASE_URL` and `JWT_SECRET` must be in the process
  environment (export them or use a run config). A missing or unknown `ENV` is a boot error, and
  `ENV=prod` requires a `JWT_SECRET` of at least 32 bytes.
- `docker-compose.yml` is at the repo root (postgres:17-alpine, db/user/password `justpaint`, :5432).
- sqlc is an external CLI (`server/sqlc.yaml`; the generated code is from sqlc v1.31.1). goose is both:
  the server embeds `server/migrations/` and applies them at boot (`AUTO_MIGRATE`, default true), and
  the CLI is for manual work. There is no Makefile.

### Strict decode for small bodies, lax for documents

Auth, match-create and assist bodies use `web.DecodeJSON` (`DisallowUnknownFields`, small caps);
document-carrying bodies use `web.DecodeJSONLax` (unknown fields tolerated, 8 MiB) for forward
compatibility — the client's `thumbnail` field is accepted and ignored. Don't unify them.

### Logout is not behind `RequireAuth`

It must clear the cookie for an expired or anonymous caller too.

### Foreign resources answer 404; submit is the one 403

A non-player reading a match (or a missing or malformed id) gets `404 not_found`, as with drawings, so
existence never leaks; the check is `isPlayer` over the loaded roster. `POST /api/matches/{id}/submit`
by a non-player returns `403` (`ErrNotPlayer`), the deliberate known-existence exception in `API.md` §1.
Don't unify them.

### Middleware order: `Recover` inside `LogRequests`

`LogRequests` stores the request id in its own rebound `r` before calling `next`, and writes one access
line per request (`method`, `path`, `status`, `duration_ms`, `request_id`, `client_ip`). If `Recover`
were the outer one, its closure would hold the original `r` and every panic log would have an empty
`request_id`. `RateLimit` sits inside both. Re-check this, and `Unwrap()` below, before reordering the
chain in `main.go`.

### `TRUST_PROXY` decides what the client IP is

Rate limits, WS connection caps and the access log key on `web.ClientIP`. With `TRUST_PROXY=false` (the
default) that is `RemoteAddr`, so behind a proxy every client collapses into the proxy's IP and one
shared bucket. With `TRUST_PROXY=true` it is the rightmost `X-Forwarded-For` entry — the one our proxy
appended; a client can only prepend forged entries — and an inbound `X-Request-Id` is adopted. Set it
true only behind exactly one trusted proxy: without a proxy clients can forge the header, and with two
chained proxies (a CDN in front of a load balancer) the real client is second from the right, which is
not implemented.

### The rate limiter fails open at capacity

When its bucket map is full and every bucket is recently active, `ratelimit.Limiter.Allow` lets a new
key through untracked. Deliberate: failing closed would let anyone with enough distinct IPs lock out
every other caller's first request. The idle-bucket sweep keeps this path rare.

### Tokens must carry `exp`

`parseToken` requires an `exp` claim (`jwt.WithExpirationRequired`), and the WS session-expiry close
(`time.AfterFunc(exp)` → close `4001`) relies on it. Any new token-issuing path must stamp `exp`.

### `color.Color.RGBA()` is alpha-premultiplied 16-bit

The fake judge's ink test (`internal/judge/fake.go`) shifts `>>8` to compare with an 8-bit threshold;
the judged raster is opaque, so premultiplication does not matter there. A heuristic on semi-transparent
input must un-premultiply first. `inkThreshold = 250`, not 255, absorbs anti-aliased edges.

## Database and game logic

### Multi-write paths use a transaction; `Get` reads without a snapshot

Game writes run in a pgx transaction (`pool.Begin` → `s.q.WithTx(tx)` → `defer tx.Rollback` →
`tx.Commit`); `assemble()` takes a `*db.Queries` so it works on either the tx or the pool.
`Service.Get` runs its match → prompt → roster reads on the pool with no snapshot, so a concurrent write
can produce a torn view (e.g. `open` with a two-player roster), and the WS fan-out calls it far more
often than the REST poll. It is tolerated because the client applies state monotonically and the next
frame or poll supersedes a stale one. Wrap `Get` in a read-only transaction (or one joined query) if
that stops being true.

### A user can still stack two open matches

`CreateOrJoin` dedupes a user's own open match with a plain `SELECT` (`FindMyOpenMatch`) under Read
Committed, so two truly concurrent creates by one user can both open a match. The composite primary key
still prevents a double seat. The hard fix (advisory lock or partial unique index) is open in
`IDEAS.md`; don't file it again as a separate bug.

### Retire prompts, never delete them

Matches reference `prompt_id` forever; set `active = false`. `PickRandomActivePrompt` uses
`order by random()` — a full scan and sort, fine for the seeded pool, not for a large one.

### Submitted duel drawings are immutable through CRUD

`UpdateDrawing` and `DeleteDrawing` include `and match_id is null`, so they touch no row for a duel
drawing, and `classifyMiss` turns that miss into `ErrDuelLocked` → `409` (a missing or foreign id stays
404). Without it a player could swap in a better drawing between submitting and judging.

### The last-submit → judging flip needs the match row lock

Submit takes `GetMatchForUpdate` (`SELECT … FOR UPDATE`) at the top of its transaction. Without it two
simultaneous submits each see the other as unsubmitted, neither flips to `judging`, and the match wedges
in `drawing`.

### Deadlines rely on `now()` being the transaction start time

`GetMatchForUpdate`'s `server_now`, `StampSubmission` and `SetMatchDrawing` use plain `now()`, which is
constant for a whole transaction. A submit whose transaction began before the deadline but got the row
lock after it still counts as on time, which is intended. Don't change these to `clock_timestamp()`.

### A late submit is a plain `409 conflict`

There is no `round_expired` error code: `ErrRoundExpired` maps to `409` with the message
`"round expired"`, keeping the error-code set closed (`API.md` §3). The client treats any submit `409`
as "poll the result".

### Image A/B comes from SQL order

`GetSubmissionsForJudging` orders by `(submitted_at, user_id)`: row 0 is image A, row 1 image B, and the
game maps the judge's `A`/`B`/`tie` back to players (`GAME.md` §7.1). Changing that `ORDER BY`, or the
inner join into a left join, silently rebinds the mapping.

### Judging runs out of band and is recovered by the sweeper

The final submit starts `judgeMatch` after its transaction commits (`dispatchJudging` →
`judging.tryGo`, bounded by `JUDGE_CONCURRENCY`) on a fresh background context. The pass is not tracked
by the shutdown `WaitGroup`, so a crash or shutdown can cut it short; the stuck-judging sweep
(`sweeper.go`) re-fires it up to `maxJudgeAttempts` and then closes the match as `done` / `aborted`
without Elo (`GAME.md` §4.1). Start a pass through `dispatchJudging`, never a bare `go`.

### `runJudging`'s status check is not a lock

`runJudging` reads the status without a lock and bails unless it is `judging`; that does not stop two
passes rendering and judging at once. `persistResult` re-takes `GetMatchForUpdate` and re-checks
`judging` inside its write transaction: the first to lock commits, the other bails. That is what makes a
sweeper re-fire safe alongside a live pass. Keep it when refactoring.

### `JudgePassBudget` must fit the judge's retry envelope

A pass (two renders plus the judge call with retries) runs under `game.JudgePassBudget` (60s). The
judge makes up to 3 attempts (`JUDGE.md` §7), so at `JUDGE_TIMEOUT=10s` it alone may need 30s; a
tighter budget silently cancels the last retry. `main.go` warns at boot when `3 × JUDGE_TIMEOUT` does
not fit.

### The sweeper's `SKIP LOCKED` lists are only candidates

`ListExpiredDrawingMatches` and the other list queries run in autocommit, so their
`FOR UPDATE SKIP LOCKED` lock ends when the SELECT returns. Exactly-once comes from each handler
(`resolveExpiredMatch`, `refireJudging`, `abortJudging`, `reapOpenMatch`) opening its own transaction,
re-locking with `GetMatchForUpdate` and re-checking the status. `drain()` stops once a phase handles
fewer than a full batch, so a batch that keeps failing does not hot-loop against an unhealthy database.

### Ratings move by an atomic delta: the match lock is per match, not per user

The match `FOR UPDATE` lock serializes per match; it cannot protect a `users` row shared by two matches
that resolve concurrently. So `ApplyRatingDelta` runs `rating = rating + delta returning rating`
(Postgres re-reads the row after taking the lock, so both deltas land), never an absolute `SET`.
`writeFinalResult`, the single Elo write site for judged and forfeit results, derives
`rating_before`/`rating_after` from that `RETURNING` and writes players sorted by `user_id` so a
concurrent rematch cannot deadlock. Each delta is still sized from a pre-match rating, as in any Elo
rating period. Covered by `internal/game/rating_db_test.go`.

### `GetMatchPlayerDrawing` binds viewer and target positionally

`$1 = match_id`, `$2 = target_user_id` (whose drawing is returned), `$3 = viewer_user_id` (the caller,
the membership gate). Swapping `$2`/`$3` in the SQL or reordering the generated params struct moves the
gate onto the path-supplied user — an IDOR that compiles. The call site uses field names, and
`TestPlayerDrawing_DB` (`internal/game/reveal_test.go`) catches a flip with its "non-member refused for
a submitted target" case; it needs `DATABASE_URL`.

### The leaderboard recomputes the whole ladder per request

`ListTopRatings` joins users, match players and matches, then groups and sorts everything before
`limit`, with no covering index or cache. For now it is bounded by the clamped `?limit` (`API.md` §11)
and the client's 30s `staleTime`; add an index, a materialized rank or a cache before the ladder grows.

## WS realtime (`internal/ws`)

### Every `ResponseWriter` wrapper needs `Unwrap()`

`websocket.Accept` finds `http.Hijacker` by walking `Unwrap() http.ResponseWriter`. Any middleware
wrapper without it in front of the WS route makes the handshake fail with `501` — the reason
`statusRecorder.Unwrap` exists (`internal/platform/web/middleware.go`).

### A context deadline on `Read`/`Write` closes the whole connection

In coder/websocket, any context expiry passed to `Read` or `Write` closes the connection, not just that
call. So a rolling idle timeout cannot be built from short per-`Read` contexts (the first timeout kills a
healthy socket), and a received pong does not extend an in-flight `Read`. That is why `readPump` reads on
the connection's lifetime context and `heartbeatLoop` evicts idle connections (close `4002`) from a
separate ticker checking a wall-clock `lastActive`. Re-read this before changing either.

### `Ping` needs a concurrent `Read` loop

`(*websocket.Conn).Ping` waits for a pong that only a running `Read`/`Reader` observes. `heartbeatLoop`
relies on `readPump`; a code path that stops reading (`CloseRead`, or not starting `readPump`) makes
every `Ping` hang until its context expires. `Ping`, `Write` and `Close` may run concurrently with each
other and with `Read`; two concurrent `Read`s may not.

### The heartbeat uses protocol pings

The server probes with native WS ping/pong, which browsers answer in the network stack: no client code,
and no background-tab timer throttling. The client's own app-level ping (`WS_PING_MS = 25000` in
`PlayView.vue`) can be throttled, and `WS_READ_IDLE_TIMEOUT` must still clear that 25s cadence with
margin.

### Connection caps: `0` means unlimited in code, so config refuses it

`newConnLimiter` treats a cap ≤ 0 as unlimited (tests rely on that). `config.Load` rejects
`WS_MAX_CONNS` or `WS_MAX_CONNS_PER_IP` below 1, a per-IP cap above the global one, and a heartbeat that
is not shorter than the idle timeout.

### Frame order is not guaranteed

Per-viewer `match_state`/`result` frames are built and sent on a goroutine per event
(`fanoutPerViewer`), so frames can arrive out of order, even to one client. Correctness depends on the
client applying them monotonically (`applyRoster`/`applyResult`: a terminal phase never regresses),
never on send order.

### Presence frames reach the sender too

When a user's first socket joins, `handleRegister` broadcasts `opponent_connected` to the whole room,
including that socket. Clients ignore presence frames carrying their own `userId` (`handleWsFrame`).

## AI assist, providers and budgets

### Set `Retry-After` before `web.Error`

`web.Error` calls `WriteHeader`, and `net/http` silently drops headers set after that. The assist
handler's per-user limiter and the per-IP tiers (`platform/web/ratelimit.go`) both set the header first;
any new 429 path must too.

### Two different limits both answer `429`

`/api/matches`, `/api/drawings`, `/api/practice` and `/api/guess` share the per-IP write tier (burst 30,
one token per 2s, easy to trip behind NAT), which clears in seconds, and sit behind the daily AI-call
budget (`internal/aibudget`), which clears tomorrow. The difference is `Retry-After`: the rate tiers
always send it, the budget never does (`API.md` §3.1). The client uses `isBudgetExhausted()` (`429`
without `Retry-After`) for "come back tomorrow"; `isRateLimited()` only says that some limit refused.
This works only while the API is same-origin, because `Retry-After` is not CORS-safelisted.

### Bill the ledger where the provider call happens

A duel writes its player rows at round start but its provider row in `Service.enterJudging`, which wraps
every `SetMatchJudging` call. Billing at round start over-counted forfeits (no judge call) and
under-counted stuck-judging re-fires (one row for up to three passes). Route any new transition into
`judging` through `enterJudging`.

### The per-user AI cap is enforced by the insert, and is still not exact

`RecordAICallUnderCap` is one statement whose `WHERE` counts the window and writes all rows or none;
zero rows is the refusal. An unlocked read-then-insert let 24 of 25 simultaneous requests through a cap
of 2. Under Read Committed two concurrent statements can still see the same count, so it is close, not
exact. The advisory pre-check stays because it refuses before the render.

### Provider quota errors must map to the same refusal

The provider's own limit arrives as `judge.ErrQuotaExhausted` (Gemini `429 RESOURCE_EXHAUSTED`). Every
handler that calls a provider (game, practice, guess, assist) must map it to the same refusal as our own
budget; otherwise it surfaces as a `500` and the UI offers a retry that cannot succeed. Our ceiling sits
below the provider's, but that is not a guarantee.

### Keep the provider's error text

When wrapping a third-party failure, put context in front of their message, never in place of it. A
retired model id came back as `404 NOT_FOUND` naming its replacement — a one-run diagnosis only because
the text survived. Model ids are pinned (not a `-latest` alias, since the judge decides ratings) and live
in configuration (`GEMINI_MODEL`, `AI_MODEL_PER_KIND`) because they get retired.

### Structured output can be truncated and still parse

When a list answer runs out of output tokens, the API closes the JSON so it parses; the last element is
half-written (a rect with `x`, `y` and nothing else) and fails validation as if the model drew badly. A
caller that asks for a list must set `MaxOutputTokens` (`judge.GeminiJSONRequest`; assist uses 8192)
and check for `judge.GeminiFinishTruncated` before blaming the answer. Verdicts are a few scalars and
leave it unset.

### At temperature 0 the schema is the grammar

Greedy decoding follows any repetitive continuation the schema allows. `NUMBER` coordinates produced
`440.000000…` until truncation, and optional fields let the model repeat the same three-field stub shape.
Assist coordinates are therefore `INTEGER` (`geminiCoordType`) and every shape property is required
(`geminiAssistRequiredShapeFields`), with meaningless values dropped on the way in. For a new
structured-output seam, ask of every field whether a degenerate continuation is representable.

### The assist `note` is untrusted text

It is model prose steered by user input. Render it as text, never as HTML.

## Testing and local tooling

### DB-backed Go tests skip silently without `DATABASE_URL`

The `*_db_test.go` files, `internal/game/reveal_test.go`, `internal/drawings/roundtrip_test.go` and the
migrate tests `t.Skip` unless `DATABASE_URL` points at a reachable, migrated database, so a plain
`go test ./...` passes without running the SQL-level checks. CI provides a database.

### `go test -race` does not run on a stock Windows toolchain

`-race` needs cgo and a C compiler, which a plain Go install on Windows lacks. CI (Linux) runs
`go test -race ./...`, so a local `go test ./...` is a weaker gate: check CI after pushing, or install
mingw-w64 and set `CGO_ENABLED=1`.

### Check which process owns `:8080`

Killing a backgrounded `go run` kills the wrapper, not the compiled binary, which keeps serving the old
build; the next `go run` fails to bind (easy to miss in a background log) while `/readyz` answers 200
from the old code. Another local process can also bind `[::1]:8080`, which wins for `localhost`;
`http://127.0.0.1:8080` forces IPv4. Before trusting a local result, check the listener
(`Get-NetTCPConnection -LocalPort 8080 -State Listen`, or `netstat -ano`) and stop that pid. Running a
`go build -o` binary directly avoids the wrapper.

### Manual duel testing needs two players and a clean pool

A duel reaches `judging` only with the Go backend up and two authenticated players. Matchmaking
auto-joins the oldest waiting `open` match, and open matches are reaped only after 10 minutes, so
leftovers from a previous run hijack new players. Between runs, on a local database:
`delete from match_players; delete from drawings where match_id is not null; delete from matches;`

### Hidden tabs pause `requestAnimationFrame`

In a hidden tab (`document.hidden`) rAF does not run: Konva's `batchDraw` never flushes (canvas pixels
are stale), Vue `<Transition>`s never finish, the rAF-throttled cursor readout never appears, and code
that awaits rAF hangs. CSS transitions freeze too, so `getComputedStyle` can report an old value while
the custom properties already changed. Verify canvas pixels in a visible tab or through the Node render
worker; DOM-level checks still work.

### Escape on a modal `<dialog>` cannot be tested through automation

A synthesized Escape reaches JS listeners but does not trigger the browser's own close action, even on
a bare `<dialog>`. A dialog that closes on Escape under automation does so through an app `keydown`
handler. Test Escape by hand, and test dismissal in automation through the close button and the
backdrop.

### Only a rendered browser catches overlapping chrome

Type checks, Vitest (happy-dom has no layout), stylelint and axe never see pixel geometry, so absolutely
positioned chrome can overlap with every gate green. `npm run test:layout -w @justpaint/web`
(`tests/layout/chrome-overlap.spec.ts`, Playwright at eleven viewports) checks the shell's bottom
regions; like `test:a11y` it needs a dev server, so it is a local gate, not a CI one. Key a breakpoint to
the condition, not the device class: the zoom-island lift uses `width <= 1200px` because the
shrink-to-fit toolbar reaches the island below about 1169px, not only on phones.

### Tool scripts must resolve packages, not assume the root `node_modules`

npm hoisting changes between installs; `check-styles.mjs` failed with `ENOENT` when `@oriui/css` moved
into `apps/web/node_modules`. Use `createRequire(import.meta.url).resolve('<pkg>/package.json')`.
