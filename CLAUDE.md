# justpaint — Claude Code guide

> Auto-loaded each session. Keep concise; deep detail lives in `docs/`.

## What this is
justpaint is a web drawing app being rebuilt as a **portfolio + learn-Go** project. The **north star is a game**: an AI-judged drawing duel — two players draw the same prompt, an ML "judge" scores similarity to the prompt and picks a winner (ratings, later teams/tournaments). A free-draw editor is a supporting mode.

Greenfield — no production data to preserve; schema/format may be redesigned freely.

## North star & scope
- **Primary:** the game (`/play`) — async duel first (create → both draw → submit → judge → result); live realtime later (Go WS hub).
- **Supporting:** free-draw editor (`/draw`) — editor + save/load only, kept deliberately minimal. The same editor powers both modes.
- **External:** the ML judge is built by a collaborator (his own ML). We define the contract + a fake impl; we do NOT build it.

## Stack
- **Frontend:** Vue 3 + Vite + Pinia + TanStack Query. Rendering on **Konva** (+ `perfect-freehand` for brush quality). We do NOT hand-write a render engine. Component lib: **oriui** (owner's lib; replacing `vueinjar`).
- **Backend:** **Go** (replacing the old NestJS) — net/http + pgx + sqlc + goose + golang-jwt + bcrypt + slog; coder/websocket for realtime. One Postgres.
- **Storage:** drawings as a **vector document (jsonb)**; rendered PNGs (judge/thumbnails) to object storage later.

## Target architecture (monorepo)
```
packages/document/   # vector doc schema + (de)serialize + render→PNG  (the contract)
packages/editor/     # Konva + perfect-freehand: tools, layers, history, export
apps/web/            # Vue app: /draw (free) + /play (game)
server/              # Go modular monolith: auth + drawings + game + WS hub + judge client
docs/                # specs & agreements (source of truth)
```
Reusability = package boundaries (`editor` consumed by both modes), not separate repos. Modular monolith, not microservices. The friend's judge is the only external service.

## Current state — IMPORTANT (not yet restructured)
The repo is still the OLD structure, mid-migration:
- `client/` — old Vue raster paint → becomes `apps/web` + extracted packages.
- `server/` — old **NestJS** backend → to be **REPLACED** by Go. Do not invest in refactoring it.

The target structure above does **not exist yet**. Phase 0 (specs) is complete — Phase 1 (Go backend + minimal Konva editor) is next. See `docs/ROADMAP.md`.

## Hard rules / gotchas
- Don't deeply refactor the NestJS backend — it's being thrown away for Go.
- Stand on Konva; don't reinvent rendering. Own the document model, rent the renderer.
- The judge is external — code only the interface + a fake; never block on the ML.
- Keep `/draw` minimal (editor + save/load); all product energy goes to the game. Avoid the two-products trap.
- The old code is learning-grade with known red flags (plaintext passwords, fake layers, broken axios error handling, JWT empty-secret fallback, Triangle draws a rect). Don't copy its patterns into the rewrite.

## Commands (current repo)
- Frontend: in `client/` — `npm run dev` (Vite), `npm run build`, `npm run types` (vue-tsc), `npm run lint:all`.
- Backend (old NestJS): in `server/` — `npm run start:dev`. Go backend: TBD (Phase 1).

## Conventions
- TS strict; avoid `any`; explicit types at boundaries. Vue 3 Composition API + `<script setup>`.
- Go: idiomatic, stdlib-first, `internal/` packages, table-driven tests, errors wrapped with context.
- Commits: present tense, one logical change each.

## Docs map
Source of truth lives in `docs/`:
- `docs/DECISIONS.md` — decision log (the "why"). **[exists]**
- `docs/DOCUMENT-FORMAT.md` — the keystone vector-doc schema (v1). **[exists]**
- `docs/ROADMAP.md` — phases & current status (durable tracker). **[exists]**
- `docs/ARCHITECTURE.md` — topology & boundaries. **[exists]**
- `docs/API.md` — HTTP contract + WS sketch (`jp_session` cookie, error envelope, DoS caps). **[exists]**
- `docs/JUDGE.md` — judge contract (the agreement with the ML collaborator). **[exists]**
- `docs/GAME.md` — match lifecycle, canvas, ratings. **[exists]**

**Phase 0 (specs) is complete.** See `docs/ROADMAP.md` for the current phase.
