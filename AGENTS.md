# AGENTS.md

Tool-agnostic entry point for AI coding agents. **[`CLAUDE.md`](CLAUDE.md) is the source of truth** —
this file is a short map; read it (and the docs it links) before non-trivial work. Claude Code reads
`CLAUDE.md`; other tools read this.

## What this is

justpaint — a **portfolio + learn-Go** project whose north star is an **AI-judged drawing duel**
(two players draw the same prompt; an external ML judge scores similarity and picks a winner). A
free-draw editor (`/draw`) is a supporting mode. Greenfield; Phases 1–2 (Go backend, vector editor)
are done and **Phase 3 (the game) is in progress** — the async-duel loop runs end-to-end in the
backend (create/join → submit → out-of-band judging → result + Elo, on swappable render/judge seams);
the pixel-authoritative Node render worker + the `/play` page are next. A **Go + TS monorepo** (npm
workspaces for the TS side; the Go service is separate).

## Setup & commands

```bash
npm install                       # wires packages/* + apps/* workspaces
npm run build|types|test          # fan out to all TS workspaces
npm run dev -w @justpaint/web     # Vite dev server on :7777

# Go backend (in server/) — needs DATABASE_URL + JWT_SECRET exported (no .env autoload)
docker compose up -d              # Postgres (postgres:17-alpine, :5432) — run from repo root
go run ./cmd/server               # API on :8080 (the vite /api proxy target)
go build ./... && go vet ./... && go test ./...
```

goose (migrations, `server/migrations/`) and sqlc (`sqlc generate`, `server/sqlc.yaml`) are external
CLIs, not Go module deps. There is no CI yet — run the gates locally.

## Layout

```
packages/document/   @justpaint/document — vector-doc schema + validate + serialize (the contract)
packages/editor/     @justpaint/editor — Konva + perfect-freehand: pure tools, render, Editor controller
apps/web/            @justpaint/web — Vue 3 SPA: /draw (free), /legacy (parked raster app); /play = Phase 3
server/              Go modular monolith: auth + drawings + judge/render seams + game (full async duel: create/join/submit/judge/result; WS hub = rest of Phase 3)
docs/                specs — the source of truth
```

## Conventions agents get wrong (full set in CLAUDE.md)

- **Contract parity:** the vector-document format lives in **two validators** — `packages/document`
  (TS) and `server/internal/document` (Go). They must stay 1:1: every invariant on both sides, DoS
  caps identical to `docs/API.md`, mirrored test tables. A format change touches the spec AND both
  validators AND both test tables together.
- **Dependency direction:** `document` imports nothing; `editor` imports only `document` + Konva +
  perfect-freehand (never Vue/router/API); app logic stays in `apps/web`.
- **Trust boundary:** client PNGs/thumbnails are advisory — anything judged or persisted is derived
  server-side from the vector document. Ownership is scoped in every query; a foreign row answers
  **404**, not 403.
- **The judge is a seam:** code only the `Judge` interface + a fake; never build or block on the ML.
- **Keep `/draw` minimal** (editor + save/load). Don't grow it into a second product.
- **Two API clients coexist:** use the native-`fetch` client (`src/core/api/drawings.ts` +
  `useSessionStore`) for new work; the legacy axios client serves only `/legacy`.
- **Commits:** Conventional Commits, present tense, one logical change, git author **Leonid**; branch
  + `--no-ff` merge for multi-commit work (see `CONTRIBUTING.md`).

## Docs map

| File | What |
| --- | --- |
| [`CLAUDE.md`](CLAUDE.md) | **Source of truth** — scope, stack, commands, rules |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | branch / commit / merge workflow |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | phases + durable status (the real status source) |
| [`docs/DECISIONS.md`](docs/DECISIONS.md) | key decisions + rationale |
| [`docs/DOCUMENT-FORMAT.md`](docs/DOCUMENT-FORMAT.md) | the keystone vector-doc schema (v1) |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | topology & boundaries |
| [`docs/API.md`](docs/API.md) | HTTP contract (cookie, envelope, caps, pagination) |
| [`docs/JUDGE.md`](docs/JUDGE.md) / [`docs/GAME.md`](docs/GAME.md) | judge contract / match lifecycle |
| [`docs/REVIEW.md`](docs/REVIEW.md) | the per-change review bar |
| [`docs/NOTES.md`](docs/NOTES.md) | non-obvious implementation gotchas |
| [`docs/IDEAS.md`](docs/IDEAS.md) | non-blocking backlog |

## Orchestration

The main session is the **orchestrator** (plans, runs gates, verifies live, wires shared files,
records findings into `docs/NOTES.md` / `docs/DECISIONS.md`). Read-only review lenses live in
`.claude/agents/` (`jp-contract-parity`, `jp-security`, `jp-go`, `jp-frontend`, `jp-scope-guard`,
`jp-docs-reviewer`, `jp-design-reviewer`), each hunting one dimension of [`docs/REVIEW.md`](docs/REVIEW.md). Backend and
frontend share no files and both validate against the frozen contract, so they fan out on parallel
branches; integration is serialized through the orchestrator.
