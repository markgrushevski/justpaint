# justpaint — Claude Code guide

> Auto-loaded each session. Keep concise; deep detail lives in `docs/`. When this disagrees with
> `docs/ROADMAP.md` (the durable status tracker) or the code, the code wins — fix this file.

## What this is
justpaint is a web drawing app rebuilt as a **portfolio + learn-Go** project. The **north star is a game**: an AI-judged drawing duel — two players draw the same prompt, an ML "judge" scores similarity to the prompt and picks a winner (ratings, later teams/tournaments). A free-draw editor is a supporting mode.

Greenfield — no production data to preserve; schema/format may be redesigned freely.

## North star & scope
- **Primary:** the game (`/play`) — async duel first (create → both draw → submit → judge → result); live realtime later (Go WS hub). **In progress — Phase 3:** the async-duel loop runs end-to-end — the backend **and** the navigable `/play` page (live against `/api/matches`: create/auto-join → poll roster → submit → poll verdict → result + Elo), behind swappable render/judge seams; the authoritative Node render worker is wired (`RENDER_MODE=node`). Remaining: object storage for the opponent-canvas reveal, then live WS realtime.
- **Supporting:** free-draw editor (`/draw`) — editor + save/load only, kept deliberately minimal. The same editor powers both modes. **Live today.**
- **External:** the ML judge is built by a collaborator (his own ML). We define the contract + a fake impl; we do NOT build it.
- **Planned:** AI **inside the product** (text drawing commands, AI inpainting, canvas co-author) — the portfolio differentiator; see `docs/IDEAS.md` "AI inside the product" + `DECISIONS.md` 2026-07-04.

## Stack
- **Frontend:** Vue 3 + Vite + Pinia + TanStack Query. Rendering on **Konva** (+ `perfect-freehand` for brush quality). We do NOT hand-write a render engine. Component lib: **oriui** — the owner's own library, installed from npm (`@oriui/{vue,css,headless}` `1.0.0-alpha.10`, the three in lockstep); in use on `/draw` (replaced `vueinjar`).
- **Backend:** **Go 1.26** — net/http (stdlib, no framework) + pgx/v5 + sqlc + golang-jwt/v5 + bcrypt + slog. One Postgres. goose and sqlc are **external CLIs** (not Go deps). `coder/websocket` for realtime is Phase 3 (not yet a dependency).
- **Storage:** drawings as a **vector document (jsonb)**; rendered PNGs (judge/thumbnails) to object storage later.

## Monorepo (this now exists)
```
packages/document/   # vector doc schema + (de)serialize + validate + fit + freehand pins  (the contract)
packages/editor/     # Konva + perfect-freehand: pure tools, toKonva, renderToStage/PNG, Editor controller
packages/render/     # headless Node render worker (reuses editor renderToStage; node-canvas; esbuild-bundled) — the authoritative judged raster
apps/web/            # Vue app: /draw (free). /play is Phase 3.
server/              # Go modular monolith: auth + drawings + judge/render seams + game (full async duel: create/join/submit/judge/result; WS hub = rest of Phase 3)
docs/                # specs & agreements (source of truth)
```
npm workspaces (`packages/*` + `apps/*`); the Go service is separate. Reusability = package boundaries (`editor` consumed by both modes), not separate repos. Modular monolith, not microservices. The friend's judge is the only external service.

## Current state
Phase 0 (specs), **Phase 1 (Go backend + minimal editor), and Phase 2 (real vector editor) are done** — see `docs/ROADMAP.md`. **Phase 3 (the game) is in progress:** the **async-duel loop runs end-to-end**, backend **and** frontend — `internal/game` (create/auto-join → submit → out-of-band judging → result + Elo, on the `internal/judge` + `internal/render` seams) **and the navigable `/play` page** wired live to `/api/matches` (`core/api/matches.ts` + poll loop; `feat/play-api-loop`). The authoritative **Node render worker** is wired (`RENDER_MODE=node`). Next: **object storage** (the opponent-canvas reveal — `judgedImageUrl` is null until then), then **live WS realtime**.
- `/draw` is a real vector editor: real layers, command-based undo/redo, fit-to-viewport/zoom, oriui design system, save/load via TanStack Query, PNG export — all on the v1 document format, editor logic entirely in `packages/editor`.
- The full round-trip works live: register → draw with every tool → save → reload → load the same drawing back, as a vector document through Postgres jsonb.
- The **async duel loop runs end-to-end** (`/api/matches`, `internal/game`): create/auto-join → both draw → submit (validate + 1080² check, persist, `drawing → judging` on the last submit) → out-of-band judging → result with winner + reason + Elo (K=32). The **render is real**: `RENDER_MODE=node` renders the authoritative judged raster off the client via `packages/render` (the Node Konva worker reusing the editor's `renderToStage`); `RENDER_MODE=stub` (default) is a zero-dep in-process stand-in. The **judge is still a seam** (`internal/judge` FakeJudge; HTTP judge later). See `docs/GAME.md` / `docs/API.md §8`.
- The old red-flag patterns (plaintext passwords, JWT empty-secret fallback, token in localStorage, Triangle-draws-a-rect, PNG-snapshot history) are **structurally gone** — the throwaway raster app that carried them (`/legacy`) was **deleted 2026-07-02** (`chore/remove-legacy`; salvaged UX ideas live in `docs/IDEAS.md`, code recoverable from git history). Don't reintroduce them.
- **The API client is native `fetch`** (`src/core/api/drawings.ts` + `useSessionStore`, cookie `jp_session`) — the old axios/localStorage-Bearer client went with the legacy app. Use the fetch client + `useSessionStore`.

## Hard rules / gotchas
- Stand on Konva; don't reinvent rendering. Own the document model, rent the renderer.
- The judge is external — code only the `Judge` interface + a fake; never block on the ML. It gets pre-rendered PNGs; it never parses our document.
- **The document contract lives in two validators** (`packages/document` TS + `server/internal/document` Go) that must stay 1:1 — every invariant on both sides, DoS caps identical to `docs/API.md`, test tables mirrored. A format change lands in the spec AND both validators AND both test tables together.
- **Dependency direction** (ARCHITECTURE §3): `document` imports nothing; `editor` imports only `document` + Konva + perfect-freehand (never Vue/router/API); app logic stays in `apps/web`.
- **Trust boundary**: client PNGs/thumbnails are advisory; anything judged or persisted is derived server-side from the vector document. Ownership is scoped in every query — a foreign row answers **404**, never 403.
- Keep `/draw` focused (editor + save/load, no feature creep) but **polished** — the 2026-07-04 UX-first pass (ROADMAP Phase 3) gates the `/play` UI. Product energy goes to the game; avoid the two-products trap.
- The authoritative judged raster is rendered **off the client** (`packages/render`, `RENDER_MODE=node`) from the validated vector document via the editor's own `renderToStage` — never a Go rasterizer (would diverge from the pinned `FREEHAND_VERSION`), never a client PNG.

## Commands
- **Whole repo (root):** `npm run build` / `npm run types` / `npm run test` — fan out to all TS workspaces (`--workspaces --if-present`).
- **Frontend:** `npm run dev -w @justpaint/web` (Vite on **:7777**), `npm run build -w @justpaint/web`, `npm run types -w @justpaint/web` (vue-tsc), `npm run lint:all -w @justpaint/web`. Or use the `.claude/launch.json` `web` config (preview MCP).
- **Packages:** `npm run test -w @justpaint/document` / `-w @justpaint/editor` (Vitest); `npm run build -w @justpaint/document` (tsc → `dist/`). **Footgun:** `apps/web` imports the packages' built `dist/`, so after editing package `src` you must rebuild the package (no HMR across the boundary — see `docs/NOTES.md`).
- **Go backend (in `server/`):** `go run ./cmd/server` (listens on **:8080**, the vite proxy target); `go build ./...`, `go vet ./...`, `go test ./...`. Requires `DATABASE_URL` + `JWT_SECRET` **exported in the environment** — the server does **not** auto-load `.env` (copy `.env.example` and export, or use an IDE run config). Judge raster defaults to the in-process stub; for the **authoritative** render set `RENDER_MODE=node` + `RENDER_CLI=<abs path>/packages/render/dist/render.mjs` (needs `node` on PATH + `npm run build -w @justpaint/render` first).
- **DB:** `docker compose up -d` at the repo root (postgres:17-alpine on :5432). Migrate with the **goose** CLI against `server/migrations/`; regenerate query code with **sqlc generate** (`server/sqlc.yaml`) — both are external CLIs, not wired into `go run`.

## Conventions
- TS strict; avoid `any`; explicit types at package boundaries. Vue 3 Composition API + `<script setup>`. (`apps/web/tsconfig.json` does not extend `tsconfig.base.json` and omits `noUncheckedIndexedAccess` — the packages are stricter than the app.)
- Go: idiomatic, stdlib-first, `internal/` packages, table-driven tests, errors wrapped with context (`%w`).
- Commits: Conventional Commits, present tense, one logical change each; branch + `--no-ff` merge for multi-commit work. See `CONTRIBUTING.md`.

## Docs map
Source of truth lives in `docs/` (each doc owns one thing and cross-references the rest):
- `docs/ROADMAP.md` — phases & current status (durable tracker; the real status source).
- `docs/DECISIONS.md` — decision log (the "why").
- `docs/DOCUMENT-FORMAT.md` — the keystone vector-doc schema (v1).
- `docs/ARCHITECTURE.md` — topology & boundaries.
- `docs/API.md` — HTTP contract (owns the `jp_session` cookie, error envelope, status map, DoS caps, pagination).
- `docs/JUDGE.md` — judge contract (the agreement with the ML collaborator).
- `docs/GAME.md` — match lifecycle, canvas, ratings.
- `docs/REVIEW.md` — the per-change review bar (contract parity, security, scope).
- `docs/NOTES.md` — non-obvious implementation gotchas (read first; append what you learn).
- `docs/IDEAS.md` — non-blocking backlog (deferred hardening & good-ideas-later).
- `CONTRIBUTING.md` — branch / commit / merge workflow. `AGENTS.md` — tool-agnostic entry map.

## Working with agents
The main session is the **orchestrator**: it plans, runs the gates, verifies live, wires shared files (route/barrel/migration numbering), and records findings into `docs/NOTES.md` / `docs/DECISIONS.md`. Read-only review lenses live in `.claude/agents/` — `jp-contract-parity`, `jp-security`, `jp-go`, `jp-frontend`, `jp-scope-guard`, `jp-docs-reviewer`, `jp-design-reviewer`; each hunts one dimension against `docs/REVIEW.md`. Backend (`server/`) and frontend (`packages/` + `apps/`) share no files and both validate against the frozen contract, so they fan out on parallel branches; integration stays serialized through the orchestrator.
