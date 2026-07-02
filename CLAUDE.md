# justpaint — Claude Code guide

> Auto-loaded each session. Keep concise; deep detail lives in `docs/`. When this disagrees with
> `docs/ROADMAP.md` (the durable status tracker) or the code, the code wins — fix this file.

## What this is
justpaint is a web drawing app rebuilt as a **portfolio + learn-Go** project. The **north star is a game**: an AI-judged drawing duel — two players draw the same prompt, an ML "judge" scores similarity to the prompt and picks a winner (ratings, later teams/tournaments). A free-draw editor is a supporting mode.

Greenfield — no production data to preserve; schema/format may be redesigned freely.

## North star & scope
- **Primary:** the game (`/play`) — async duel first (create → both draw → submit → judge → result); live realtime later (Go WS hub). **Not built yet — Phase 3.**
- **Supporting:** free-draw editor (`/draw`) — editor + save/load only, kept deliberately minimal. The same editor powers both modes. **Live today.**
- **External:** the ML judge is built by a collaborator (his own ML). We define the contract + a fake impl; we do NOT build it.

## Stack
- **Frontend:** Vue 3 + Vite + Pinia + TanStack Query. Rendering on **Konva** (+ `perfect-freehand` for brush quality). We do NOT hand-write a render engine. Component lib: **oriui** — the owner's own library, installed from npm (`@oriui/{vue,css,headless}` `1.0.0-alpha.2`, the three in lockstep); in use on `/draw` (replaced `vueinjar`).
- **Backend:** **Go 1.26** — net/http (stdlib, no framework) + pgx/v5 + sqlc + golang-jwt/v5 + bcrypt + slog. One Postgres. goose and sqlc are **external CLIs** (not Go deps). `coder/websocket` for realtime is Phase 3 (not yet a dependency).
- **Storage:** drawings as a **vector document (jsonb)**; rendered PNGs (judge/thumbnails) to object storage later.

## Monorepo (this now exists)
```
packages/document/   # vector doc schema + (de)serialize + validate + fit + freehand pins  (the contract)
packages/editor/     # Konva + perfect-freehand: pure tools, toKonva, renderToPNG, Editor controller
apps/web/            # Vue app: /draw (free) + /legacy (parked old raster app). /play is Phase 3.
server/              # Go modular monolith: auth + drawings (game + WS hub + judge client = Phase 3)
docs/                # specs & agreements (source of truth)
```
npm workspaces (`packages/*` + `apps/*`); the Go service is separate. Reusability = package boundaries (`editor` consumed by both modes), not separate repos. Modular monolith, not microservices. The friend's judge is the only external service.

## Current state
Phase 0 (specs) and **Phase 1 (Go backend + minimal Konva editor) are done** — see `docs/ROADMAP.md`. **Phase 2 (real vector editor: layers, undo/redo, fit/zoom) is next.**
- The full round-trip works live: register → draw with every tool → save → reload → load the same drawing back, as a vector document through Postgres jsonb.
- The old red-flag patterns (plaintext passwords, JWT empty-secret fallback, token in localStorage, Triangle-draws-a-rect, PNG-snapshot history) are **structurally gone** in the new path. They survive only in the parked legacy raster app behind `/legacy` (`apps/web/src/TheApp.vue` + `src/modules/canvas/**`, the old axios client `src/core/api/api.ts`, `useUserStore`) — do **not** copy those patterns forward.
- **Two API clients coexist under `@core`**: the current native-`fetch` client (`src/core/api/drawings.ts` + `useSessionStore`, cookie `jp_session`) for all new work, and the legacy axios client (localStorage Bearer) for `/legacy` only. Always use the fetch client + `useSessionStore`.

## Hard rules / gotchas
- Stand on Konva; don't reinvent rendering. Own the document model, rent the renderer.
- The judge is external — code only the `Judge` interface + a fake; never block on the ML. It gets pre-rendered PNGs; it never parses our document.
- **The document contract lives in two validators** (`packages/document` TS + `server/internal/document` Go) that must stay 1:1 — every invariant on both sides, DoS caps identical to `docs/API.md`, test tables mirrored. A format change lands in the spec AND both validators AND both test tables together.
- **Dependency direction** (ARCHITECTURE §3): `document` imports nothing; `editor` imports only `document` + Konva + perfect-freehand (never Vue/router/API); app logic stays in `apps/web`.
- **Trust boundary**: client PNGs/thumbnails are advisory; anything judged or persisted is derived server-side from the vector document. Ownership is scoped in every query — a foreign row answers **404**, never 403.
- Keep `/draw` minimal (editor + save/load); all product energy goes to the game. Avoid the two-products trap.
- Don't invest in the `/legacy` code — it's the throwaway raster app, kept only as a reference/fallback.

## Commands
- **Whole repo (root):** `npm run build` / `npm run types` / `npm run test` — fan out to all TS workspaces (`--workspaces --if-present`).
- **Frontend:** `npm run dev -w @justpaint/web` (Vite on **:7777**), `npm run build -w @justpaint/web`, `npm run types -w @justpaint/web` (vue-tsc), `npm run lint:all -w @justpaint/web`. Or use the `.claude/launch.json` `web` config (preview MCP).
- **Packages:** `npm run test -w @justpaint/document` / `-w @justpaint/editor` (Vitest); `npm run build -w @justpaint/document` (tsc → `dist/`). **Footgun:** `apps/web` imports the packages' built `dist/`, so after editing package `src` you must rebuild the package (no HMR across the boundary — see `docs/NOTES.md`).
- **Go backend (in `server/`):** `go run ./cmd/server` (listens on **:8080**, the vite proxy target); `go build ./...`, `go vet ./...`, `go test ./...`. Requires `DATABASE_URL` + `JWT_SECRET` **exported in the environment** — the server does **not** auto-load `.env` (copy `.env.example` and export, or use an IDE run config).
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
The main session is the **orchestrator**: it plans, runs the gates, verifies live, wires shared files (route/barrel/migration numbering), and records findings into `docs/NOTES.md` / `docs/DECISIONS.md`. Read-only review lenses live in `.claude/agents/` — `jp-contract-parity`, `jp-security`, `jp-go`, `jp-frontend`, `jp-scope-guard`, `jp-docs-reviewer`; each hunts one dimension against `docs/REVIEW.md`. Backend (`server/`) and frontend (`packages/` + `apps/`) share no files and both validate against the frozen contract, so they fan out on parallel branches; integration stays serialized through the orchestrator.
