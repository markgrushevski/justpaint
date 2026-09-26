# justpaint ships as ONE image on ONE origin: the Go binary serves /api, the
# WebSocket and the built SPA together. That is not a packaging preference — the
# session cookie is httpOnly+SameSite and the WS handshake enforces same-origin
# with no CORS headers anywhere, so splitting the frontend onto a second host
# would break every authenticated request.
#
# The image carries a Node runtime as well, because the authoritative judged
# raster is rendered by the editor's own Konva path in a Node worker
# (packages/render). The Go service spawns it as a local child process, so both
# runtimes must sit on the same filesystem.
#
# Host-agnostic on purpose: everything is env-driven, so this same image runs on
# Render, Fly, a VPS or plain `docker run`.

# ---- 1. Frontend + render worker -------------------------------------------
FROM node:24-bookworm-slim AS web
WORKDIR /app

# Copy manifests first so `npm ci` caches across source-only changes.
COPY package.json package-lock.json ./
COPY packages/editor/package.json packages/editor/
COPY packages/render/package.json packages/render/
COPY apps/web/package.json apps/web/
RUN npm ci --no-audit --no-fund

COPY tsconfig.base.json ./
COPY packages/ packages/
COPY apps/ apps/
# Fans out to every workspace: the document + editor dist/ the app compiles
# against, the SPA bundle, and the esbuild-bundled render worker.
RUN npm run build

# ---- 2. Go service ----------------------------------------------------------
FROM golang:1.26-bookworm AS server
WORKDIR /src

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
# Static binary: the runtime stage is a Node image, not the Go one.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/justpaint-server ./cmd/server

# ---- 3. Runtime -------------------------------------------------------------
FROM node:24-bookworm-slim AS runtime

# ca-certificates: outbound TLS (an external judge, an LLM API).
# The rest are node-canvas's shared libraries. Its prebuilt binary usually
# carries its own, but a missing one surfaces only at the first judging — as a
# match stuck in `judging`, not as a boot error — so they are installed here.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        libcairo2 \
        libpango-1.0-0 \
        libpangocairo-1.0-0 \
        libjpeg62-turbo \
        libgif7 \
        librsvg2-2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# `canvas` is a native addon: the render worker deliberately leaves it external
# (packages/render/build.mjs), so the runtime needs it installed. The version is
# read from the package manifest rather than pinned twice — one source of truth.
COPY packages/render/package.json /tmp/render-package.json
RUN CANVAS_VERSION="$(node -p "require('/tmp/render-package.json').dependencies.canvas")" \
    && npm install --omit=dev --no-audit --no-fund --prefix /app "canvas@${CANVAS_VERSION}" \
    && rm -f /tmp/render-package.json /app/package.json /app/package-lock.json

COPY --from=server /out/justpaint-server /usr/local/bin/justpaint-server
COPY --from=web /app/apps/web/dist/ /app/web/
COPY --from=web /app/packages/render/dist/render.mjs /app/render/render.mjs

# Defaults that describe THIS image's layout. Everything else (DATABASE_URL,
# JWT_SECRET) must come from the environment — the server fails fast without them.
ENV ADDR=:8080 \
    ENV=prod \
    STATIC_DIR=/app/web \
    RENDER_MODE=node \
    RENDER_CLI=/app/render/render.mjs \
    RENDER_NODE_BIN=node \
    JUDGE_CONCURRENCY=2 \
    LOG_LEVEL=info

USER node
EXPOSE 8080

# No shell form: the binary is PID 1 and receives SIGTERM directly, which is what
# the graceful shutdown (drain HTTP, then the hub and the sweeper) depends on.
CMD ["justpaint-server"]
