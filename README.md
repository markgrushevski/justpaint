# justpaint

A web drawing app built around a game: an **AI-judged drawing duel**. Two players draw the same prompt,
a judge scores each drawing against it and picks a winner, and Elo ratings feed a leaderboard. A
free-draw vector editor (`/draw`) shares the same editor and hosts the AI features that need a canvas
without a clock.

- **Duel** (`/play`) — async matches with a server-authoritative deadline, forfeit and abandon rules,
  and live WebSocket updates.
- **Practice** (`/practice`) — one prompt, one drawing, one score, no opponent.
- **Editor** (`/draw`) — layers, undo/redo, zoom, save/load, PNG export; **assist** turns a text prompt
  into shapes previewed before you accept them, and **"what did I draw?"** asks the model to guess.

Drawings are stored as a versioned **vector document** (Postgres `jsonb`), never as pixels. Anything
judged is rendered server-side from that document by the same code the browser editor uses.

## Stack

- **Frontend:** Vue 3, Vite, Pinia, TanStack Query; canvas on **Konva** with **perfect-freehand**;
  components from [oriui](https://github.com/markgrushevski/oriui), a library developed alongside this
  project.
- **Backend:** Go 1.26 — stdlib `net/http`, `pgx/v5`, `sqlc`, `golang-jwt`, `bcrypt`, `slog`,
  `coder/websocket`. One binary, one Postgres.
- **Render worker:** Node, reusing the editor's Konva code through `node-canvas`.

## Quickstart

Needs Node 24, Go 1.26, Docker, and the **goose** CLI (plus **sqlc** if you change queries).

```sh
docker compose up -d                  # Postgres 17 on :5432
goose -dir server/migrations postgres "postgres://justpaint:justpaint@localhost:5432/justpaint?sslmode=disable" up

# the server does not load .env — export what it needs (full list: server/.env.example)
export ENV=dev
export DATABASE_URL="postgres://justpaint:justpaint@localhost:5432/justpaint?sslmode=disable"
export JWT_SECRET="$(openssl rand -base64 48)"

(cd server && go run ./cmd/server)    # API on :8080

npm install
npm run dev -w @justpaint/web         # app on :7777, proxies /api to :8080
```

Open <http://localhost:7777>.

### Fakes by default

Every external dependency has an offline stand-in, so dev and CI need no keys:

| Variable | Default | Real option |
|---|---|---|
| `RENDER_MODE` | `stub` — an ink block sized by stroke count, not the drawing | `node` + `RENDER_CLI=/abs/path/packages/render/dist/render.mjs` (build it with `npm run build -w @justpaint/render`) |
| `JUDGE_MODE` | `fake` — scores ink coverage, never reads the prompt | `gemini` (vision model), or `http` for the external ML judge ([docs/JUDGE.md](docs/JUDGE.md)) |
| `ASSIST_MODE` | `fake` — returns the same canned drawing | `gemini` |

The Gemini options need `GEMINI_API_KEY`. Pair a real judge with `RENDER_MODE=node`; on the stub it scores ink
blocks, and the server warns at boot. The free Gemini tier allows about 20 requests per day per
model, so every AI call goes through a daily ledger with a per-player and a global ceiling
(`AI_DAILY_PER_USER`, `AI_DAILY_GLOBAL`).

## Commands

| Command | Does |
|---|---|
| `npm run build` / `types` / `test` | build, typecheck, test every TS workspace |
| `npm run format` / `format:check` | prettier (skips `docs/` and `server/`) |
| `npm run lint:all -w @justpaint/web` | prettier, stylelint, eslint, contrast and stylesheet checks |
| `npm run test:a11y -w @justpaint/web` | axe in a real browser (needs the dev server) |
| `npm run test:layout -w @justpaint/web` | floating chrome never overlaps, at 11 viewports (needs the dev server) |
| `cd server && go test ./...` | Go tests; CI also runs them with `-race` against a real Postgres |

CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs the same gates on every push to `main`
and every pull request.

## Layout

```
packages/editor/     the vector document types and the Konva editor: tools, layers, undo/redo, rendering
packages/render/     headless render worker for the judged raster
apps/web/            the Vue app
server/              the Go service
docs/                contracts and decisions
```

`server/internal/` is a modular monolith: `auth`, `drawings`, `document` (the document
validator), `game` (match lifecycle, deadline, Elo), `practice`, `guess`, `judge`, `assist`,
`aibudget` (the AI-call ledger), `ratings`, `render`, `ws`, `db` (sqlc), `platform` (config, HTTP,
Postgres, logging).

## Deploy

One image, one origin. The session cookie is `httpOnly` + `SameSite` and the WebSocket handshake checks
the origin, so the Go binary serves the API, the WebSocket and the built SPA together. The image also
carries Node for the render worker.

```sh
docker build -t justpaint .
docker run --rm -p 8080:8080 \
  -e JWT_SECRET="$(openssl rand -base64 48)" \
  -e DATABASE_URL="postgres://user:pass@host:5432/justpaint" \
  justpaint
```

- **Migrations run at boot** from the embedded `server/migrations/`, behind an advisory lock. Set
  `AUTO_MIGRATE=false` if something else owns the schema.
- **`DATABASE_URL` must be a Postgres connection string.** On Supabase use the **Session pooler**
  (`…pooler.supabase.com:5432`): the direct host is IPv6-only on the free plan, and the transaction pooler
  (6543) drops the prepared statements pgx relies on.
- **Probes:** `GET /healthz` (process up) and `GET /readyz` (503 while Postgres is unreachable or
  unmigrated).
- [render.yaml](render.yaml) targets Render. On the free tier the service sleeps after ~15 minutes idle
  (keep it warm by pinging `/readyz`), and a free Render Postgres expires after 30 days, so use an
  external one.

## Docs

| Doc | Owns |
|---|---|
| [ARCHITECTURE](docs/ARCHITECTURE.md) | topology, package boundaries, dependency direction |
| [DOCUMENT-FORMAT](docs/DOCUMENT-FORMAT.md) | the vector document schema |
| [API](docs/API.md) | the HTTP and WebSocket contract |
| [GAME](docs/GAME.md) | match lifecycle, canvas, ratings, AI budget |
| [JUDGE](docs/JUDGE.md) | the judge contract |
| [ASSIST](docs/ASSIST.md) | the AI-assist operation contract |
| [DESIGN-SYSTEM](docs/DESIGN-SYSTEM.md) | how the UI uses oriui |
| [DECISIONS](docs/DECISIONS.md) | why things are the way they are |
| [NOTES](docs/NOTES.md) | non-obvious gotchas — read before changing things |
| [ROADMAP](docs/ROADMAP.md) | what is left to build |
| [IDEAS](docs/IDEAS.md) | the backlog beyond it |
| [ISSUES-INNER](docs/ISSUES-INNER.md) / [ISSUES-OUTER](docs/ISSUES-OUTER.md) | known problems, ours / upstream |
| [REVIEW](docs/REVIEW.md) | what a change is reviewed against |
| [CONTRIBUTING](CONTRIBUTING.md) | branches, commits, local gates |

## License

[MIT](LICENSE)
