# justpaint

justpaint is a web drawing app. The north star is a game: an **AI-judged drawing duel** — two
players draw the same prompt, a judge scores each drawing's similarity to the prompt and picks a
winner, with Elo ratings and a leaderboard tracking players over time. A free-draw vector editor
(`/draw`) is the supporting mode; it shares the exact same editor that powers the duel.

## Status

| Phase | Goal | Status |
|---|---|---|
| 0 | Specs — lock the contracts before code | done |
| 1 | Go backend (auth + drawings CRUD) + a minimal Konva editor | done |
| 2 | Frontend refactor — the real vector editor, oriui | done |
| 3 | The game — async duel first, then live realtime | done |
| 4 | Stretch — ratings, AI assist, teams/tournaments, replay | in progress |

Full detail and history: [`docs/ROADMAP.md`](docs/ROADMAP.md).

Concretely shipped:

- the **vector document format** — versioned, renderer-agnostic, stored as Postgres `jsonb`;
- the **Konva + perfect-freehand editor** — real layers, command-based undo/redo, fit-to-viewport
  and zoom, PNG export;
- **cookie-session auth** (bcrypt + JWT, httpOnly `jp_session`) and **drawings CRUD**, scoped so a
  foreign row 404s instead of leaking a 403;
- the full **async duel**: create/auto-join → both players draw → submit → out-of-band judging →
  result;
- a **server-authoritative round deadline**, with forfeit when exactly one player submits and
  abandon when neither does;
- **live WebSocket realtime** — match state, opponent submissions, judging, and results pushed to
  both players as they happen, with the REST poll loop kept as a reconciliation fallback;
- **Elo ratings and a leaderboard** (`GET /api/leaderboard`);
- an **authoritative server-side render worker** — the judged raster is produced by the same
  Konva + perfect-freehand projection the browser editor uses, run headless in Node, so the judge
  scores real rendered pixels rather than a client-supplied image;
- **AI Assist Phase A** — a text prompt turns into a validated batch of document operations,
  previewed as a ghost layer before the user accepts or rejects it.

## Current limits

- **The judge is a stand-in.** `FakeJudge` scores each image by ink coverage — the fraction of
  non-background pixels — and does not read the prompt at all. The real judge is being built by an
  external collaborator; it plugs into the `Judge` interface (`server/internal/judge`) against the
  contract in [`docs/JUDGE.md`](docs/JUDGE.md), with no change to the game loop.
- **AI Assist's model backend is scaffolded, not live.** `ASSIST_MODE=fake` — deterministic,
  no API key required — is the default and what ships. `ASSIST_MODE=anthropic` wires the config
  and fails fast without an `ANTHROPIC_API_KEY`, but has no live model call behind it yet.
- **No object storage yet.** The result screen's opponent-canvas reveal renders the opponent's
  vector document client-side through a membership-gated endpoint instead; `judgedImageUrl` stays
  `null` until a storage layer is added.

## Stack

- **Frontend** — Vue 3 + Vite + Pinia + TanStack Query; canvas rendering on **Konva** with
  **perfect-freehand** for brush quality; components from **oriui** (the owner's own library,
  `@oriui/vue` + `@oriui/css` + `@oriui/headless`, installed from npm in lockstep). No hand-written
  render engine.
- **Backend** — **Go 1.26**, stdlib `net/http` (no framework) + `pgx/v5` + `sqlc` + `golang-jwt/v5`
  + `bcrypt` + `slog`, plus `coder/websocket` for realtime. One binary, one Postgres.
- **Storage** — drawings are a **vector document** stored as Postgres `jsonb`, never raster;
  rendered PNGs (the judged raster, thumbnails) are produced on demand by the render worker.

## Monorepo layout

```
packages/document/   vector-document schema, validation, (de)serialize, render contract — the keystone
packages/editor/     Konva + perfect-freehand editor: tools, layers, undo/redo, renderToStage/PNG
packages/render/     headless Node render worker — reuses the editor's renderToStage via node-canvas
apps/web/            Vue 3 SPA — /draw (free editor) and /play (the duel)
server/              Go modular monolith — one binary, internal/ modules below
docs/                specs and decisions — the source of truth
```

`server/internal/`:

| Module | Owns |
|---|---|
| `auth` | signup/login, bcrypt, JWT issue/verify, the `jp_session` cookie, middleware |
| `drawings` | CRUD over the vector document (`jsonb`); validates on write; ownership-scoped |
| `document` | the Go half of the vector-document validator (mirrors `packages/document`) |
| `game` | match lifecycle — create/join, submit, judging, result, round deadline, forfeit/abandon |
| `render` | the `Renderer` seam — `StubRenderer` and `NodeRenderer` (spawns `packages/render`) |
| `judge` | the `Judge` interface + `FakeJudge` |
| `assist` | the `Assist` interface + `FakeAssist` + a scaffolded `AnthropicAssist` |
| `ratings` | the read-only leaderboard (Elo aggregate + sort over match players) |
| `ws` | the `coder/websocket` hub pushing match-room state in realtime |
| `db` | sqlc-generated typed queries |
| `platform` | shared infra — the Postgres pool, HTTP router, config, logging |

## Quickstart

Needs **Node ≥ 22**, **Go ≥ 1.26**, **Docker**, and the external CLIs **goose** (migrations) and
**sqlc** (query codegen, only if you change queries).

```sh
# 1. Postgres (postgres:17-alpine on :5432)
docker compose up -d

# 2. apply migrations
goose -dir server/migrations postgres "postgres://justpaint:justpaint@localhost:5432/justpaint?sslmode=disable" up

# 3. required env — the server does NOT auto-load .env, export it directly
#    (see server/.env.example for the full list)
export ENV=dev   # mandatory: dev relaxes the Secure cookie flag for http://localhost
export DATABASE_URL=postgres://justpaint:justpaint@localhost:5432/justpaint?sslmode=disable
export JWT_SECRET=$(openssl rand -base64 48)

# 4. backend — API on :8080
cd server && go run ./cmd/server

# 5. frontend (separate terminal, from the repo root) — Vite on :7777, proxies /api -> :8080
npm install
npm run dev -w @justpaint/web
```

Open `http://localhost:7777` — it lands on `/draw`. Register, draw, save/load a drawing, or head to
`/play` for a duel.

**Optional: the authoritative render worker.** `RENDER_MODE` defaults to `stub`, a zero-dependency
in-process stand-in. To render the real judged raster instead:

```sh
npm run build -w @justpaint/render          # needs node on PATH
export RENDER_MODE=node
export RENDER_CLI=/absolute/path/to/packages/render/dist/render.mjs
```

## Commands

| Where | Command | Does |
|---|---|---|
| root | `npm run build` | builds every TS workspace (`packages/*` + `apps/*`, `--if-present`) |
| root | `npm run types` | typechecks every TS workspace (`tsc` / `vue-tsc`) |
| root | `npm run test` | runs Vitest across every TS workspace |
| root | `npm run format` / `npm run format:check` | prettier write/check, scoped by `.prettierignore` (`docs/`, `*.md`, and `server/` are excluded — hand-authored / gofmt-owned) |
| `apps/web` | `npm run dev -w @justpaint/web` | Vite dev server on `:7777` |
| `apps/web` | `npm run lint:all -w @justpaint/web` | prettier + stylelint + eslint + a contrast check |
| `apps/web` | `npm run test:a11y -w @justpaint/web` | Playwright + axe-core accessibility tests |
| `packages/document`, `packages/editor` | `npm run test -w @justpaint/<name>` | package-level Vitest suites |
| `server/` | `gofmt -l .` / `go vet ./...` / `go test ./...` / `go build ./...` | format check, vet, test, build the Go service |

CI runs these same gates on every push to `main` and every pull request
([`.github/workflows/ci.yml`](.github/workflows/ci.yml)); see [`docs/REVIEW.md`](docs/REVIEW.md)
for the judgment layer on top of them.

## Docs map

| Doc | Owns |
|---|---|
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | phases and durable status — the real status source |
| [`docs/DECISIONS.md`](docs/DECISIONS.md) | the decision log — the "why" behind each call |
| [`docs/DOCUMENT-FORMAT.md`](docs/DOCUMENT-FORMAT.md) | the keystone vector-document schema (v1) |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | system topology, package boundaries, dependency direction |
| [`docs/API.md`](docs/API.md) | the HTTP contract — routes, the `jp_session` cookie, error envelope, status map, DoS caps, pagination, the WS wire protocol |
| [`docs/JUDGE.md`](docs/JUDGE.md) | the judge contract — request/response shape, the positional `winner`, tie rule, raster spec |
| [`docs/GAME.md`](docs/GAME.md) | match lifecycle, the canonical game canvas, prompt pinning, the ratings sketch |
| [`docs/ASSIST.md`](docs/ASSIST.md) | the AI-assist `Op` contract, the `internal/assist` seam, the ghost-preview UX |
| [`docs/DESIGN-ASSIST-PHASE-A.md`](docs/DESIGN-ASSIST-PHASE-A.md) | the AI Assist Phase A implementation brief (shipped; retained as design history) |
| [`docs/DESIGN-PHASE3-LIVE.md`](docs/DESIGN-PHASE3-LIVE.md) | the round-deadline + WS-realtime design (shipped; retained as design history) |
| [`docs/REVIEW.md`](docs/REVIEW.md) | the per-change review bar — contract parity, security, scope |
| [`docs/DESIGN-SYSTEM.md`](docs/DESIGN-SYSTEM.md) | how the UI consumes oriui — the frontend chrome contract |
| [`docs/NOTES.md`](docs/NOTES.md) | non-obvious implementation gotchas |
| [`docs/IDEAS.md`](docs/IDEAS.md) | the non-blocking backlog |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | branching, commit style, and the local gates |
| [`AGENTS.md`](AGENTS.md) | a concise, tool-agnostic map of the repo |

## License

[MIT](LICENSE).
