# AGENTS.md

Tool-agnostic entry point for AI coding agents. **[`CLAUDE.md`](CLAUDE.md) is the source of truth** —
this file is a short map; read it (and the docs it links) before non-trivial work. Claude Code reads
`CLAUDE.md`; other tools read this.

## What this is

justpaint — a web drawing app whose north star is an **AI-judged drawing duel**
(two players draw the same prompt; an external ML judge scores similarity and picks a winner). A
free-draw editor (`/draw`) is a supporting mode. Greenfield; **Phases 1–3 are done** (Go backend,
vector editor, the game) — the async-duel loop runs end-to-end (create/join → submit → out-of-band
judging → result + Elo), a server-authoritative round deadline with forfeit/abandon, the `/play` page
live against `/api/matches`, and live WS realtime (`internal/ws`) all shipped; the authoritative
render worker (`packages/render`, `RENDER_MODE=node`) is live and the judge is a seam with a real
impl behind it (`JUDGE_MODE=gemini`; `fake` is the dev/CI default). **Phase 5** (public release) is
done. **Phase 4** (AI assist, ratings/leaderboard, single-player practice and the per-kind AI budget
shipped; realtime hardening, teams/tournaments, replay, and the collaborator's ML judge remain) and
**Phase 6** (post-launch UI) are in progress. A **Go + TS monorepo** (npm workspaces
for the TS side; the Go service is separate).

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
CLIs, not Go module deps. CI runs the same gates on push/PR (`.github/workflows/ci.yml`) — run them locally first.

## Layout

```
packages/document/   @justpaint/document — vector-doc schema + validate + serialize (the contract)
packages/editor/     @justpaint/editor — Konva + perfect-freehand: pure tools, renderToStage, Editor controller
packages/render/     @justpaint/render — headless Node render worker (reuses editor renderToStage; node-canvas; esbuild-bundled)
apps/web/            @justpaint/web — Vue 3 SPA: /draw (free); /play = the duel; /practice, /leaderboard
server/              Go modular monolith: auth + drawings + judge/render/assist seams + game (full async duel: create/join/submit/judge/result) + practice + guess + the ai_calls budget + WS realtime hub (internal/ws)
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
- **The judge is a seam:** the collaborator's `Judge` contract is frozen and we never build or block
  on his ML — `FakeJudge`, `HTTPJudge` and `GeminiJudge` all sit behind the one interface, selected
  by `JUDGE_MODE`. Our own vision seams (`Critic` for practice, `Guesser` for guess) are separate
  interfaces, never a widening of his.
- **Keep `/draw` focused but polished.** It is the editor plus save/load plus the AI-in-product
  surfaces (assist, "guess what I drew"), which land here because it is the only canvas with no
  opponent and no clock. Anything with a score, a ladder or an opponent belongs in the game.
- **The API client is native `fetch`** (`src/core/api/drawings.ts` + `useSessionStore`); the old
  axios/localStorage-Bearer client went with the deleted `/legacy` app — don't reintroduce it.
- **Commits:** Conventional Commits, present tense, one logical change, git author **Leonid**; branch
  + `--no-ff` merge for multi-commit work (see `CONTRIBUTING.md`).

## Docs map

| File | What |
| --- | --- |
| [`CLAUDE.md`](CLAUDE.md) | **Source of truth** — scope, stack, commands, rules |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | branch / commit / merge workflow |
| [`docs/ORIENTATION.md`](docs/ORIENTATION.md) | the owner's reading path + operator's manual (Russian); load-bearing files, hidden invariants, runbook, symptom→where-to-look |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | phases + durable status (the real status source) |
| [`docs/DECISIONS.md`](docs/DECISIONS.md) | key decisions + rationale |
| [`docs/DOCUMENT-FORMAT.md`](docs/DOCUMENT-FORMAT.md) | the keystone vector-doc schema (v1) |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | topology & boundaries |
| [`docs/API.md`](docs/API.md) | HTTP contract (cookie, envelope, caps, pagination) |
| [`docs/JUDGE.md`](docs/JUDGE.md) / [`docs/GAME.md`](docs/GAME.md) | judge contract / match lifecycle |
| [`docs/REVIEW.md`](docs/REVIEW.md) | the per-change review bar |
| [`docs/NOTES.md`](docs/NOTES.md) | non-obvious implementation gotchas |
| [`docs/IDEAS.md`](docs/IDEAS.md) | non-blocking backlog |
| [`docs/ISSUES-INNER.md`](docs/ISSUES-INNER.md) / [`docs/ISSUES-OUTER.md`](docs/ISSUES-OUTER.md) | known problems — ours to fix here / a dependency's upstream |

## Orchestration

The main session is the **orchestrator** (plans, runs gates, verifies live, wires shared files,
records findings into `docs/NOTES.md` / `docs/DECISIONS.md`). Read-only review lenses live in
`.claude/agents/` (`jp-contract-parity`, `jp-security`, `jp-go`, `jp-frontend`, `jp-scope-guard`,
`jp-docs-reviewer`, `jp-design-reviewer`), each hunting one dimension of [`docs/REVIEW.md`](docs/REVIEW.md). Backend and
frontend share no files and both validate against the frozen contract, so they fan out on parallel
branches; integration is serialized through the orchestrator.
