# justpaint

A web drawing app rebuilt as a **portfolio + learn-Go** project. The north star is a **game**: an
AI-judged drawing duel — two players draw the same prompt, an ML "judge" scores each drawing's
similarity to the prompt and picks a winner. A free-draw editor is a supporting mode.

> **Status:** Phase 1 done — a Go backend (cookie-session auth + drawings CRUD, storing each drawing
> as a vector document in Postgres jsonb) and a minimal Konva editor that round-trips a drawing
> end-to-end. The game (`/play`) is Phase 3. See [`docs/ROADMAP.md`](docs/ROADMAP.md).

## Stack

- **Frontend** — Vue 3 + Vite + Pinia + TanStack Query; rendering on **Konva** + `perfect-freehand`;
  components from **oriui**. Drawings are a **vector document**, not raster PNGs.
- **Backend** — **Go 1.26** (net/http + pgx + sqlc + goose + golang-jwt + bcrypt + slog), one
  Postgres. The ML judge is an external service (built by a collaborator); we own only the contract
  and a fake impl.

## Layout

```
packages/document/   vector-doc schema + validate + serialize (the contract, TS)
packages/editor/     Konva + perfect-freehand editor (tools, render, controller)
apps/web/            Vue 3 SPA — /draw (free editor); /play (the game) is Phase 3
server/              Go modular monolith — auth + drawings
docs/                specs & design decisions (the source of truth)
```

## Run it

```sh
# 1. install (npm workspaces)
npm install

# 2. Postgres
docker compose up -d

# 3. backend — export DATABASE_URL + JWT_SECRET first (see server/.env.example),
#    apply migrations with goose, then:
cd server && go run ./cmd/server        # API on :8080

# 4. frontend (separate terminal)
npm run dev -w @justpaint/web           # Vite on :7777 (proxies /api -> :8080)
```

Open http://localhost:7777 — you land on `/draw`, where you can register, draw, and save/load a
drawing through the Go API.

## Docs

The design lives in [`docs/`](docs/): [ROADMAP](docs/ROADMAP.md) (phases + status),
[DECISIONS](docs/DECISIONS.md) (the why), [DOCUMENT-FORMAT](docs/DOCUMENT-FORMAT.md) (the keystone
schema), [ARCHITECTURE](docs/ARCHITECTURE.md), [API](docs/API.md), [JUDGE](docs/JUDGE.md),
[GAME](docs/GAME.md). Contributors: [`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`CLAUDE.md`](CLAUDE.md).
