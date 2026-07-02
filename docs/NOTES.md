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
- **Migration 00001 already creates `prompts` / `matches` / `match_players`** (the game tables), but
  no Go code queries them yet — their presence is not an implemented feature (Phase 3).
- **Logout is deliberately NOT behind `RequireAuth`** — it must clear the cookie even for an expired/
  anonymous caller. Don't wrap it in `protect()`.
- **Auth bodies use strict decode** (`DisallowUnknownFields`, 64 KiB); **document bodies use lax
  decode** (unknown fields tolerated, 8 MiB) for forward-compat. Don't unify them; the client
  `thumbnail` field is intentionally accepted-and-ignored.
- **Only `internal/document` has Go tests.** auth/drawings/db/config/web/postgres have zero coverage —
  their correctness was verified manually (curl/UI), per the ROADMAP. Grow tests where you touch.
- **A detached background `go run` on Windows can be reaped without diagnostics** (observed exit code
  4, perfectly clean log). Symptom in the web app: save/load fails with the `network` `ApiError`. Just
  restart the server — there's nothing to debug in Go. (Internet scanners also hit a locally-exposed
  `:8080` — stray 404s like `/en/reviews` are noise.)

## Konva / editor / render determinism

- **Konva keeps every `Stage` in a module-global registry until `stage.destroy()`.** Dropping the Vue
  ref alone leaks the stage and its `<canvas>` elements. `Editor.destroy()` exists for this — DrawView
  calls it in `onBeforeUnmount`, and every `loadDocument()` destroys+rebuilds the stage. Any new host
  of `Editor` must call `destroy()` on unmount.
- **Pointer coords come from `stage.getRelativePointerPosition()`** (transform-aware), never
  `pageX - offsetLeft` (the old DPR bug). A future fit-to-viewport/zoom must scale the Konva **stage**,
  never CSS-transform the container — that silently breaks coordinate capture.
- **Per-layer isolation is a render-contract requirement**: each document layer → its own
  `Konva.Layer` (own canvas) so composite strokes (eraser `destination-out`) can't bleed across
  layers.
- **`Editor.loadDocument()` does not validate** — callers pre-validate with `parseDocument()`
  (DrawView and `drawings.get` already do). Internal editor state is trusted.
- **`renderToPNG` is browser-only** (needs a real DOM + Konva stage) and is intentionally **not**
  unit-tested (no DOM in the Vitest runner). A headless/Konva-node server render path is a separate
  future thing (Phase 3 submit).
- **`/draw` mounts a fixed 1280×720 working canvas**, deliberately diverging from the spec
  `DEFAULT_CANVAS` (1920×1080) because the editor has no fit/zoom yet (Phase 2); the wrapper just
  scrolls. Don't "fix" it to 1920×1080 without adding fit/zoom.

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
  via their built `dist/` (`"*"` → `dist/index.js`). That `dist/` is **gitignored, not committed**
  (only `vendor/oriui/**/dist/` is force-tracked) — so a **fresh clone has no package `dist` at all**
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
- **Vendored oriui is prebuilt and `file:`-linked** (`vendor/oriui/{css,headless,vue}`, all
  1.0.0-alpha.1). Treat it as an external dependency: editing vendor source needs a rebuild;
  `@oriui/css` must be imported for side effects (done in `main.ts`) or components render unstyled.
  Its committed `dist/` is force-included via `.gitignore` negations (`!vendor/oriui/**/dist/`).

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
