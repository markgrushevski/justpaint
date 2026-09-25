# justpaint — agent guide

Loaded every session. Keep it short; detail lives in `docs/`. When this file and the code disagree, the
code wins — fix this file.

## What this is

A web drawing app built around an **AI-judged drawing duel** (`/play`): two players draw one prompt, a
judge scores both, and Elo feeds a leaderboard. `/practice` is the single-player mode. `/draw` is the
free editor and hosts the AI features that need a canvas without a clock (assist, "what did I draw?").
The ML judge is an external service: this repo owns the `Judge` contract and its impls (`fake`, `http`,
`gemini`), never the model.

## Stack

- **Frontend:** Vue 3, Vite, Pinia, TanStack Query; Konva + perfect-freehand; oriui
  (`@oriui/{vue,css,headless}`, pinned to one exact version, all three in lockstep).
- **Backend:** Go 1.26 — stdlib `net/http`, pgx/v5, sqlc, golang-jwt/v5, bcrypt, slog, coder/websocket.
  One Postgres; goose migrations are embedded and applied at boot.
- **Render worker:** `packages/render`, Node, reusing the editor's `renderToStage`.

```
packages/document   the vector document contract (TS)
packages/editor     Konva editor — imports only document, Konva, perfect-freehand
packages/render     headless worker that renders the judged raster
apps/web            the Vue app
server              Go modular monolith (internal/*)
docs                contracts and decisions
```

## Hard rules

- **Stand on Konva.** Own the document model; never hand-write a render engine.
- **The document contract lives in two validators** — `packages/document` (TS) and
  `server/internal/document` (Go) — kept 1:1 with `docs/DOCUMENT-FORMAT.md` and the caps in
  `docs/API.md`. A format change touches the spec, both validators and both test tables together.
- **Dependency direction:** `document` ← `editor` ← `apps/web`, never back (ARCHITECTURE §3).
- **Trust boundary:** client PNGs are advisory. Anything judged or persisted is derived server-side from
  the validated document, and the judged raster comes from `packages/render` — never a Go rasterizer,
  never a client image. Every query is ownership-scoped: a foreign row answers 404, never 403.
- **Auth** is the httpOnly `jp_session` cookie; the client is native `fetch` + `useSessionStore`. No
  tokens in localStorage, no empty-secret fallback.
- **The judge is external.** Code the interface and fakes; never block on the ML service.
- **oriui is consumed, not restyled** (`docs/DESIGN-SYSTEM.md`): colors are set once in `main.css`;
  button state goes through props (`variant`, `pressed`, `disabled`, `loading`); icon actions are
  `OriButton`/`IconButton`; floating chrome is `OriSurface`, content is `OriCard`. Never override `.ori-*`
  rules; setting a public oriui token (`--ori-color`, `--ori-size-action`) on an element is the escape hatch.
- **`/draw` stays focused.** A feature belongs there if it makes drawing better or more fun; anything
  with a score, a ladder or an opponent belongs to the game.

## Commands

- **Root:** `npm run build` · `types` · `test` · `format` / `format:check` (prettier skips `docs/` and
  `server/`).
- **Web:** `npm run dev -w @justpaint/web` (:7777) · `lint:all` / `lint:ci` · `test:a11y` ·
  `test:layout` — the last two need the dev server running.
- **Packages:** `apps/web` imports each package's built `dist/`, so rebuild after editing a package's
  `src` (`npm run build -w @justpaint/<name>`).
- **Go** (in `server/`): `go run ./cmd/server` (:8080) needs `ENV`, `DATABASE_URL` and `JWT_SECRET`
  exported — there is no `.env` autoload. `gofmt -l .` · `go vet ./...` · `go test ./...`.
- **DB:** `docker compose up -d`; `goose` and `sqlc` (`server/sqlc.yaml`) are external CLIs.
- **CI** runs the same gates, but Go tests run with `-race`, which needs cgo and so doesn't run on a stock
  Windows toolchain. Local green is not CI green for Go — check CI after pushing.

## Conventions

- TS strict, no `any`, explicit types at package boundaries; Vue 3 `<script setup>`. `apps/web` omits
  `noUncheckedIndexedAccess`; the packages don't.
- Go: stdlib first, `internal/` packages, table-driven tests, errors wrapped with `%w`.
- Conventional Commits, one logical change each; a branch and a `--no-ff` merge for multi-commit work
  (`CONTRIBUTING.md`). No AI co-author trailers.
- Docs and comments are written for an outside developer: plain and short, no narration about who asked
  or who found what, no AI-process talk. `docs/` is hand-written and not prettier-formatted.
- `ISSUES-INNER.md` / `ISSUES-OUTER.md`: newest entry first; delete an entry in the change that resolves it.

## Docs

`docs/` is the source of truth, one owner per topic. Contracts: ARCHITECTURE, DOCUMENT-FORMAT, API,
GAME, JUDGE, ASSIST, DESIGN-SYSTEM. Then DECISIONS (why), NOTES (gotchas — read first), ROADMAP (what
is left), IDEAS (backlog), ISSUES-INNER / ISSUES-OUTER (known problems, ours / upstream — oriui reads
OUTER as its queue), REVIEW (what a change is reviewed against).

## Agents

Read-only review agents live in `.claude/agents/` (`jp-*`), one dimension each, measured against
`docs/REVIEW.md`; they report and never edit. `server/` and `packages/` + `apps/` share no files, so they
can be worked in parallel; shared wiring (routes, barrels, migration numbers) is integrated serially.
