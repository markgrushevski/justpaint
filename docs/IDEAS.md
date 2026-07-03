# Ideas & backlog (non-blocking)

> Parking lot for improvements surfaced mid-build that are **deliberately deferred** — not bugs, not Phase-1 blockers. Each item has a one-line rationale + a rough *when*. Promote an item into `ROADMAP.md` when its phase goes active. Hard, already-made trade-offs live in `DECISIONS.md`; this is the softer "good ideas, later" list.

## Auth / identity
- **Login charset/format validation** — `login` is currently validated by **length only** (3–254 chars). Add format rules: if it contains `@`, validate as an email; otherwise restrict a nickname to `[a-zA-Z0-9_.-]` (no spaces, no emoji). This is the real gap (vs. the email/nickname *split*, which isn't needed). *When:* cheap — fold into the next auth touch.
- **Optional separate `email` column** — split the sign-in handle (`login`) from a verified contact `email`, unlocking password reset, email verification, notifications. *When:* only when an email flow is actually built (YAGNI until then). Until then the single `login` (email-or-nickname, citext, case-insensitive) stands as decided.

## Observability (server)
- **Request-id correlation** — generate (or read `X-Request-Id`), attach to the slog context and echo in the response header, so every log line of one request correlates. *When:* next backend hardening pass; cheap, high value.
- **Prometheus `/metrics`** — `prometheus/client_golang`: a metrics middleware (`http_requests_total{method,route,status}`, a latency histogram, in-flight gauge), `pgxpool.Stat()` gauges, Go runtime collectors; Grafana dashboards on top. *When:* when dashboards are wanted / for portfolio depth.
- **Loki + Grafana (logs)** — the JSON-to-stdout slog is already Loki-friendly; shipping (promtail/alloy → Loki → Grafana) is an **ops** concern, not code. *When:* deploy time.
- **OpenTelemetry tracing** — traces → Tempo/Jaeger, viewable in Grafana. *When:* later; overkill for v1.

## Drawings / API
- **Rate limiting (429)** — already tracked in `DECISIONS.md` (deferred). The real remaining abuse vector for drawings (spamming valid-but-heavy documents), **not** SQL injection (queries are fully parameterized via sqlc; ownership is enforced in every `WHERE owner_id`). *When:* before any public exposure / Phase 3.
- **Server-generated `thumbnail_url` only** — when thumbnails land, store **only** a server-side object-storage URL, never a client-supplied one (avoids stored-SSRF / XSS via a poisoned URL). *When:* when thumbnails are implemented (Phase 2/3).
- **One open match per user (concurrency hardening)** — the `FindMyOpenMatch` dedupe (`DECISIONS.md` 2026-07-03) is Read-Committed **best-effort**: a truly concurrent same-user double-tap can still open two `open` matches (never a double-*seat* — the composite PK guards that). Enforce hard with `pg_advisory_xact_lock(hashtext(userID))` at the top of the create tx, or a partial unique index. **Fold into the rate-limit slice** — same abuse class, same deferral. *When:* the deferred rate-limit / pre-public-exposure pass. *(jp-go + jp-security review, feat/game-matches.)*

## Frontend / build
- ~~**Replace vendored oriui with the published npm package**~~ — **DONE** (2026-07-02): `apps/web` now installs `@oriui/{vue,css,headless}` `1.0.0-alpha.2` from npm; `vendor/oriui/` and the `.gitignore` exception are gone. See `docs/NOTES.md` (oriui is a normal npm dep now; version bumps need a Vite restart).
- **`Editor` needs automated coverage + gesture/pan abandon-hardening** — the `Editor` class (Konva
  integration: viewport/zoom/pan/`ResizeObserver`, gesture commit) has **no** unit tests; only the pure
  `view.ts` / `history.ts` / tool math is tested. Add a jsdom (or stubbed-container `Konva.Stage`) test
  exercising `fitToViewport`/`zoomBy`/pan against a mocked `getPointerPosition`. Related: an in-flight
  `gesture`/`pan` isn't cleared if a `pointerup` fires **outside** the container (Konva's pointer
  capture is per-hit-shape, not per-stage) — a window-level `pointerup`/`pointercancel` fallback would
  reset both. *When:* next editor hardening pass. *(jp-frontend review, feat/editor-fit.)*
- **Workspace package dist resolution (dev footgun)** — `apps/web` imports `@justpaint/editor` + `@justpaint/document` from their built `dist/` (their `package.json` `exports` point at `./dist`), so editing those packages' `src` requires a rebuild before the app (or `vue-tsc`) sees the change — no HMR across the package boundary. *Fix later:* a Vite alias `@justpaint/* → src` (+ matching tsconfig `paths`) or a watch build. *When:* if editor/document churn during app work gets annoying.
- ~~**Editor fit-to-viewport / zoom**~~ — **DONE** (2026-07-03, `feat/editor-fit`): the editor sizes its Konva *stage* to the container and fits the document via the stage transform (a `ResizeObserver` auto-fits; wheel zooms to cursor; middle-button pans; `Ctrl/Cmd 0/±`); `getRelativePointerPosition` stays logical at any zoom. *Remaining touch polish:* pinch-zoom + two-finger pan (see the "Zoom / pan / fit gestures" idea below).

- **SPA needs an `/api` reverse proxy in prod** — `apps/web` calls a same-origin `/api` base; dev uses the vite proxy (`vite.config.js`), but a production static host must reverse-proxy `/api` → the Go server (nginx/Caddy) or every call 404s. No committed prod config yet (`.env.production` / proxy sample). *When:* first real deployment.
- **CSRF posture is SameSite=Lax only** — state-changing endpoints lean on the `jp_session` cookie being `SameSite=Lax` (blocks the classic cross-site POST) rather than a CSRF token. Fine for the current phase; revisit (token / double-submit) if the threat model widens. *When:* before public multi-user launch.

## Orchestration / process
- **Cross-domain parallel agents** — backend (`server/`) and frontend (`packages/` + `apps/`) share no files and both validate against the **frozen contract** (`docs/DOCUMENT-FORMAT.md`), so they can run on **parallel branches**; integration (review + `--no-ff` merge + ROADMAP update) stays **serialized** through one orchestrator. Within a single domain (e.g. the editor's tools), parallelize with git-worktree isolation or sequential commits to avoid same-file conflicts. *When:* as work volume warrants; the editor tool-set is the first good fan-out candidate.

## Salvaged from the `/legacy` app (UX ideas)
The parked raster app was deleted 2026-07-02 (`chore/remove-legacy`); its raster engine is superseded by the vector editor, but these **UX/interaction patterns are worth rebuilding** on the new stack (oriui + the vector editor). Recoverable in full from git history if the exact code is ever wanted.

- **Slide-in side menu** — a fixed side panel that slides in/out via a `transform` translate with a hamburger toggle (legacy `MenuToggler`, an `OriCard` with title/body/footer slots, ~400px). Non-intrusive: it never overlays the canvas. *(The interface the owner specifically liked.)* **Where:** a file-ops / settings drawer on `/draw`, and match/lobby chrome on `/play`.
- **Three-zone responsive layout** — header (history) / center canvas / footer (tools + settings), `100dvh`, footer growing on wider screens. **Where:** a consistent `DrawView` shell.
- **Light/dark theme toggle** — cycles auto/light/dark, persists to `localStorage`, applies a class on `documentElement`, honors `prefers-color-scheme` (legacy `ThemeToggler` + `useThemesStore`). The new app has no theme switch yet. **Where:** the toolbar or the side menu.
- **Saved-drawings browser with metadata** — a scrollable card list of saved drawings showing creation date + thumbnail, instead of today's "load the most recent" button (legacy `LoadArt`). **Where:** `/draw` load flow; reused for `/play` history. (Pairs with the server-side `thumbnail_url` idea above.)
- **Copy as text / image to clipboard** — export the rendered drawing as a data-URL or a PNG blob straight to the clipboard (legacy `CopyArt`). **Where:** an editor export affordance next to Export-PNG.
- **Color/size popover controls** — a two-swatch (stroke + fill) visual that opens a popover (not a modal) with the pickers, closes on click-outside, and blocks canvas input while open so a stray stroke can't land (legacy `ColorSettings`, `@vueuse` `onClickOutside`). **Where:** a cleaner `EditorToolbar` (move color/width into a collapsible settings card).
- **Auth-aware actions** — Save disabled until signed in; an inline login↔register toggle sharing one compact space; a saved-count badge (legacy `SaveArt` / `UserProfile`). **Where:** `SessionBar` polish.

## Design & UX ideas (from similar tools & games)
Researched 2026-07-02 from drawing editors (Excalidraw, tldraw, Figma/FigJam, Photopea) and drawing-duel games (Skribbl.io, Gartic Phone, Draw Battle, Jackbox Drawful). **Recorded only — not building now.** Grouped by surface; the game items serve the north star (`/play`).

**Editor & canvas (`/draw`, `packages/editor`)**
- **Bottom-docked / floating toolbar** — thumb-reachable, doesn't eat the canvas; adapts to a compact bar on mobile (tldraw, FigJam).
- **Zoom / pan / fit gestures** — pinch-zoom, two-finger pan, wheel+shift; `Ctrl+0` fit-all, `Ctrl+1` 100%. (This is the Phase-2 fit-to-viewport work — scale the Konva **stage**.)
- **Compact color control** — a preset swatch grid (~18 colors) + recent-colors history + a "more" button to the full picker; eyedropper (Alt-click) (Photopea, Sketchful).
- **Brush-size preview** — a live circle under the cursor / next to the tool showing size + opacity before drawing (Procreate, Photoshop).
- **Layer thumbnails** — mini previews per layer in the panel (Figma, Photopea) — pairs with the server `thumbnail_url` idea.
- **Visual undo-history** — a hoverable list of recent actions; click to jump back (Photopea History, Krita).
- **Shortcuts cheat-sheet** — a `?`/`Cmd+?` modal listing keys (Excalidraw, tldraw). Extra tools: fill-bucket, more shapes.

**Game — lobby & match (`/play`, Phase 3)**
- **Minimal lobby** — big prompt text, ready-up toggle (auto-start when all ready), live player-avatar list with ready dots; host settings via sliders (rounds, seconds/round, public/private) (Skribbl, Gartic Phone).
- **Centered prompt reveal** — the prompt appears center-screen ~3s at round start, then each player draws their own canvas (Gartic Phone, Drawful).
- **Round timer bar** — top progress bar, green→orange→red under 10s, with an alert cue (Skribbl).
- **Split view for spectators** — both canvases side-by-side for the audience; each player sees only their own during the round (Draw Battle).

**Game — result & rating (`/play`, Phase 3)**
- **Animated result card** — both drawings side-by-side, the ML similarity as a 0–100% bar, the judge's one-line reason, an arrow animating to the winner (Game UI Database, LoL victory screen). A `?` tooltip expands the judge's rationale (transparency).
- **Score pop + leaderboard** — floating "+15" juice on award; a leaderboard table (nick / avatar / score / Δ) highlighting the current player; a match-summary screen with Rematch / Share (Skribbl, Jackbox).
- **One-click share** — export the round (both drawings + result) as a PNG/GIF for social (Gartic Phone album, ShareX).

**Cross-cutting — theme, responsive, feel**
- **Dark-mode toggle** — moon/sun switch; oriui already ships both themes (also in the salvaged-legacy list).
- **Mobile portrait layout** — canvas ~70%, toolbar bottom, panels collapse to icon-tabs; primary actions in the bottom third for one-handed reach.
- **Game-feel polish** — smooth 200–400ms easings, toast notifications for match events, skeleton loaders while the judge scores, optional audio cues.
- **Playful brand type** — a hand-drawn display font (Excalidraw's Virgil / Caveat) for lobby & result headings to set a fun tone; keep a clean system/`Nunito` body.

*Priority for the game MVP:* bottom toolbar · compact color picker · animated result card · round timer · match summary · dark-mode · mobile layout. Editor polish (layer thumbnails, brush preview, zoom/pan) and juice (score pop, audio, replay) come after the core loop.
