# Ideas & backlog (non-blocking)

> Parking lot for improvements surfaced mid-build that are **deliberately deferred** — not bugs, not Phase-1 blockers. Each item has a one-line rationale + a rough *when*. Promote an item into `ROADMAP.md` when its phase goes active. Hard, already-made trade-offs live in `DECISIONS.md`; this is the softer "good ideas, later" list.

## AI inside the product (north-star upgrade — planned)
User directive 2026-07-04: make justpaint an **AI-product** — AI features in the product itself, not AI as a dev tool. Priority: **high**; exact sequencing vs `/play` TBD (the `/draw` UX pass comes first either way — see `DECISIONS.md` 2026-07-04).

- **Text drawing commands** — an LLM turns natural-language commands into document operations. Our command-based history + vector document are a natural fit: the LLM emits commands, undo/redo comes free, validation guards the contract.
- **AI inpainting / draw-completion** — via an external image API; the Node render worker already produces server-side rasters to feed it. Results could come back as raster layers (needs a document extension — a version-safe additive field, format §9) or as vector strokes.
- **Canvas co-author assistant** — an agent that draws alongside the user / critiques / suggests, over the same command seam as text commands.

*Why:* closes the main portfolio gap — a project where AI is **inside** the product, not just a development tool; canvas + LLM integration is a striking demo. *When:* after the `/draw` UX pass; sequencing vs `/play` under discussion.

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

## Game — judging & render
- **Real render worker (Node Konva + perfect-freehand)** — replace `render.StubRenderer` with the pixel-authoritative worker that renders the vector document off-client using the *same* Konva + perfect-freehand path as the editor (shares `packages/document`; `import 'konva/canvas-backend'`). Swaps in behind the `render.Renderer` interface with no game-loop change. Unblocks a truthful judged raster + the result reveal image. *When:* the next Phase-3 slice — the top remaining item for the async duel. *(feat/game-submit.)*
- **Judging durability sweeper** — v1 judging is an in-process goroutine; a crash mid-judge leaves a match stuck in `judging` with no retry. Add a startup (and/or periodic) sweep that re-drives `runJudging` for any match in `judging`. `runJudging` is already idempotent, so this is safe to re-run. *When:* with the real render worker (a real render can fail/timeout, making this matter more). *(feat/game-submit.)*
- **Object storage for `judgedImageUrl`** — the result reveal shows each player the *opponent's* canvas, which the ownership-scoped `GET /api/drawings/{id}` can't serve (foreign owner → 404). So `judgedImageUrl` must point at the server-rendered PNG in object storage, authorized by match-membership. Null until this lands. Store **only** server-side URLs (never client-supplied — stored-SSRF/XSS). *When:* with the real render worker. *(Already listed under Drawings/API as the thumbnail seam — same storage.)*

## Frontend / build
- ~~**Replace vendored oriui with the published npm package**~~ — **DONE** (2026-07-02): `apps/web` now installs `@oriui/{vue,css,headless}` `1.0.0-alpha.2` from npm; `vendor/oriui/` and the `.gitignore` exception are gone. See `docs/NOTES.md` (oriui is a normal npm dep now; version bumps need a Vite restart).
- **`Editor` needs automated coverage** (~~+ gesture/pan abandon-hardening~~) — the `Editor` class (Konva
  integration: viewport/zoom/pan/`ResizeObserver`, gesture commit) has **no** unit tests; only the pure
  `view.ts` / `history.ts` / tool math is tested. Add a jsdom (or stubbed-container `Konva.Stage`) test
  exercising `fitToViewport`/`zoomBy`/pan against a mocked `getPointerPosition`. The abandon-hardening
  half **landed 2026-07-04 (`feat/draw-ux`)**: window-level `pointerup`/`pointercancel` fallbacks now
  end/abandon an in-flight `gesture`/`pan` released outside the container (Konva's pointer capture is
  per-hit-shape, not per-stage). The automated-coverage half remains open. *When:* next editor
  hardening pass. *(jp-frontend review, feat/editor-fit.)*
- **Workspace package dist resolution (dev footgun)** — `apps/web` imports `@justpaint/editor` + `@justpaint/document` from their built `dist/` (their `package.json` `exports` point at `./dist`), so editing those packages' `src` requires a rebuild before the app (or `vue-tsc`) sees the change — no HMR across the package boundary. *Fix later:* a Vite alias `@justpaint/* → src` (+ matching tsconfig `paths`) or a watch build. *When:* if editor/document churn during app work gets annoying.
- ~~**Editor fit-to-viewport / zoom**~~ — **DONE** (2026-07-03, `feat/editor-fit`): the editor sizes its Konva *stage* to the container and fits the document via the stage transform (a `ResizeObserver` auto-fits; wheel zooms to cursor; middle-button pans; `Ctrl/Cmd 0/±`); `getRelativePointerPosition` stays logical at any zoom. *Remaining touch polish:* pinch-zoom + two-finger pan (see the "Zoom / pan / fit gestures" idea below).

- ~~**`/draw` shell polish (deferred from the slice-2 review)**~~ — **DONE across 2026-07-05…08**: OriToaster/`useToast` adopted (feat/draw-legacy-menu), layer-panel glyphs are ToolIcon SVGs (feat/draw-ux-polish), and the menu went **non-modal** (legacy right-side panel, 2026-07-08 DECISIONS) so a focus trap is no longer applicable (Confirm/Shortcuts dialogs still trap).
- **SPA needs an `/api` reverse proxy in prod** — `apps/web` calls a same-origin `/api` base; dev uses the vite proxy (`vite.config.js`), but a production static host must reverse-proxy `/api` → the Go server (nginx/Caddy) or every call 404s. No committed prod config yet (`.env.production` / proxy sample). *When:* first real deployment.
- **CSRF posture is SameSite=Lax only** — state-changing endpoints lean on the `jp_session` cookie being `SameSite=Lax` (blocks the classic cross-site POST) rather than a CSRF token. Fine for the current phase; revisit (token / double-submit) if the threat model widens. *When:* before public multi-user launch.
- ~~**Light-theme outline token fails WCAG non-text contrast**~~ — **RESOLVED 2026-07-08** (feat/draw-legacy-menu): the legacy-palette pass recomputed BOTH outlines neutral and ≥3:1 (`hsl(220 5% 55%)` light — 3.08:1 on the `#f0f2f6` surface, the binding case; `hsl(0 0% 46%)` dark). The themes are symmetric again.
- **Custom canvas guides** (owner, 2026-07-07) — user-placed guide lines on `/draw`: unlimited count, horizontal AND vertical, draggable, view-only (never exported/judged — same seam as the `setCanvasBackdrop` layer). Later: snap-to-guide for the shape tools. *When:* a `/draw` power-user pass after `/play`.
- **Document background-color control** — the doc's `background` is now `null` by default (the backdrop paints paper); there's no UI to set a REAL document background color. Needs an undoable `setBackground` editor command (history.ts) + a color well in the menu's Canvas section. *When:* when someone asks for exports with a baked background.
- **Rebuild Confirm/Shortcuts dialogs on `@oriui/headless` `useDialog`** — OriDialog is still uncontrolled in alpha-6, but `useDialog` (setOpen/onOpenChange) + a native `<dialog>` + `@oriui/css/components/dialog.css` would replace ~60 lines of hand-rolled focus-trap/Esc/focus-return per dialog with platform behavior while keeping the host-driven `open` prop contract. *When:* next dialog touch.
- **Theme picker as OriMenu** — the theme chip blind-cycles auto→light→dark; an OriMenu with the three states would make them discoverable (caveat: OriMenu positions via CSS anchor — misplaced in Firefox until it ships anchor positioning). *When:* shell polish, once Firefox ships anchors (or accept the fallback).
- **Browser a11y layer** — add Playwright + `@axe-core/playwright` over `/draw` (and later `/play`): axe over the **real rendered** app, the only layer that catches rendered mis-pairings (e.g. the tooltip bug). Owner-approved; the pending next unit (see `DECISIONS.md` 2026-07-08 — the three-layer a11y stack). *When:* next a11y touch; **close this when it lands.**
- **APCA (`apca-w3`) as a supplementary advisory** — an additional contrast signal in the WCAG-3 direction (better-behaved for the vivid brand orange than the WCAG-2 ratio). Advisory only, **not a gate**. *When:* alongside the browser a11y layer, if wanted.
- **Adopt oriui `data-ori-skin=neutral`** — delegate the base palette to oriui's new `neutral` skin (`DECISIONS.md` 2026-07-08) and drop the `main.css` palette override, keeping only the desk/backdrop token + the light colored-text-tone override. *When:* a shell-token cleanup pass.
- **Publish the oriui tooltip/neutral fix, then drop justpaint's interim workarounds** — once oriui publishes `feat/neutral-skin-tooltip-contrast` (commit `bd3d847`), bump the app and remove the interim `--ori-tooltip` override + the per-cluster `placement`-only tooltip workarounds (`NOTES.md`). *When:* the next oriui bump.
- **axe `/draw` contrast triage** — the browser axe suite (`test:a11y`, `DECISIONS.md` 2026-07-08) reported these on `/draw` — all **pre-existing, allowlisted for later triage, none blocking**:
  - `.draw__brand` wordmark — #ff5500 on #f0f2f6 = **2.85:1** — brand orange as large wordmark text; a brand/`--ori-color-primary` decision (mobile hides it ≤600px, so it only affects desktop).
  - `.ori-variant_tonal` buttons ("Copy as text"/"Copy as image") — #c24100 on #e5c6b9 = **3.23:1** — oriui tonal-button token → **upstream** (`@oriui/css`).
  - selected tab `.ori-tabs__tab[aria-selected="true"]` ("Log in") — #ff5500 on #f0f2f6 = **2.85:1** — oriui selected-tab uses the primary token → **upstream**.
  - `.menu__section-title` ("File"/"Canvas") — #74777c on #f0f2f6 = **4.01:1** — app-side muted header, a near-miss vs 4.5:1; easy local fix (darken the muted tone a touch).
  - `region` (moderate, non-blocking) — the floating `/draw` chrome isn't wrapped in a `<main>`/landmark — a landmark-structure decision.

  3 of the 4 serious items are oriui-owned tokens (belong upstream in `@oriui/css`); the `.menu__section-title` near-miss and the landmark gap are the only app-side items. *When:* next a11y touch — pairs with the browser a11y layer above.

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
