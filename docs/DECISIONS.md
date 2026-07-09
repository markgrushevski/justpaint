# Decision log

Lightweight record of key decisions and their rationale, so they aren't relitigated and survive context resets / onboard new agents and collaborators. Newest first.

## 2026-07-08 — Excalidraw-inspired shell + navigable `/play` (redesign)

The owner asked to make `/draw` feel like Excalidraw and to reuse that shell for the game. Decided: borrow Excalidraw's *patterns* (a warm empty-state card with quick actions, corner discipline, tool hotkey badges, cleaner menu organization) but render them in **oriui + the brand orange** — NOT a pixel clone. A clone would fight oriui (the owner's own dogfooded library), mean maintaining two visual languages, and edge into brand mimicry. Two Excalidraw placements were **deliberately not adopted** — both owner-confirmed:

- **Kept the right-side slide-in drawer** (not Excalidraw's top-left dropdown). The drawer is *non-modal*, so the canvas — and an in-progress duel — stay live behind it; it is the natural persistent home for the `/play` profile + rating + match context, which a transient dropdown can't hold. We borrow Excalidraw's menu *organization*, not its placement.
- **Kept the bottom-center floating toolbar** (not a top bar; owner-pinned 2026-07-04). Decisive for the shared shell: `/play` owns the **top band** for the prompt banner + round timer, so a top toolbar would collide and force `/draw` and `/play` to diverge — the one thing the shared-shell rule (2026-07-04) forbids. A free top band is the strongest argument against Excalidraw fidelity here.

**Made the shared shell structural**, not just a CSS convention: extracted `apps/web/src/components/shell/EditorShell.vue` — the desk/letterbox + the Konva mount element (exposed via `defineExpose({ canvasEl })` so the composing view builds the `Editor` into it) + named region slots (`#top-left/-center/-right`, `#bottom-left/-center/-right`, `#overlay`, `#drawer`). Both `DrawView` and the new `PlayView` **compose** it; a `mode: 'draw' | 'play'` prop tunes per-mode (e.g. `/play` reclaims the top-right corner `/draw` reserves for its menu toggler). `/play` is therefore a composition of the same regions, not a fork — matching ARCHITECTURE's "reuse = package/module boundaries" and making a later `/play` split mechanical.

`/play` shipped as a **navigable scaffold on local mock state** (`waiting → drawing → submit → judging → done`); the live `/api/matches` client (create/poll/submit/result, native fetch + TanStack) is the **next unit** (`TODO(play-api)` markers), and the result reveal's opponent image waits on the deferred render worker (`judgedImageUrl`, `IDEAS.md`). Sequencing followed the 2026-07-04 plan: `/draw` UX polish (Phase A) → shared-shell extraction + `/play` scaffold (Phase B) → live duel loop. Also: the theme store is now constructed at the app root (`App.vue`) so the persisted/OS theme applies on **every** route, not only `/draw`.

## 2026-07-08 — oriui alpha-7 → alpha-10: all local oriui compensations dropped

Four back-to-back oriui bumps (`chore/oriui-alpha{7,8,9,10}`) each shipped an upstream fix that let this app **delete** a local workaround. After alpha-10 there are **zero** `.ori-*` selector overrides and **zero** interim oriui tokens in `apps/web` — consumption is clean, and every "drop when oriui publishes" NOTES/IDEAS item is closed. The three packages move in lockstep, now pinned at **`1.0.0-alpha.10`**.

- **alpha-7 — published tooltip fix; dropped the interim `--ori-tooltip` override.** oriui merged/published `feat/neutral-skin-tooltip-contrast` (the self-paired neutral bubble on the anchored, out-of-flow `.ori-anchored` primitive + viewport flip/clamp + a static arrow). The registry build renders the neutral tooltip correctly, so `main.css` no longer pins `--ori-tooltip-bg`/`--ori-tooltip-color`. *Supersedes* the same-day "override stays / not-yet-published" bullets below (and the a11y-layer correction). The directional `placement` props on the toolbar/top/zoom clusters **stay** — ordinary anchored placement now, not the old in-flow crowding workaround.
- **alpha-8 — role-as-text AA tokens; dropped the local primary-text-tone override.** oriui's colored-text button variants (outline/tonal/text) now derive an AA-safe label tone from the role token itself, so the *one* sanctioned `.ori-*` override — `main.css` darkening `.ori-button.ori-color_primary:where(_outline,_tonal,_text)` to `hsl(20 100% 38%)` on the light surface — is gone. `apps/web` now targets **no** `.ori-*` selector at all. *Supersedes* the 2026-07-07 legacy-shell entry's `hsl(20 100% 38%)` colored-text pin (below).
- **alpha-9 — one-hue relative-color refinement.** oriui refined the role-as-text derivation to a single-hue relative-color pass (tighter, hue-stable label tones); no app change beyond the bump.
- **alpha-10 — headless `applyTheme` fixes the runtime theme-toggle Chromium bug.** `@oriui/headless` now ships an `applyTheme(theme)` controller; `useThemeStore` switched from a bare `classList.toggle('ori-theme_dark')` to `applyTheme(isDark ? 'dark' : 'light')`. This works around a Chromium invalidation bug where flipping the theme class at runtime left already-styled components painting the previous theme's colours until a re-render. `main.css` still keys the justpaint brand aliases off `:root.ori-theme_dark` (unchanged); `applyTheme` just owns flipping that class.

**Net:** the salvaged-legacy light/dark toggle is live (auto→light→dark, persisted to `localStorage['jp-theme']`, applied via `applyTheme`), and justpaint carries no oriui token/selector patches — the design system is consumed as published.

## 2026-07-08 — Browser a11y layer landed

The a11y-tooling stack decided earlier today got its browser layer.

- **A11y verification architecture — three complementary layers, deliberately separate.** Each catches what the others structurally can't:
  1. **colord token-source WCAG lint** — `apps/web/scripts/check-contrast.mjs`, wired into `lint:all` (fast, no browser; checks our design-token pairs at the source).
  2. **oriui component-level axe** — happy-dom, structure-only, lives **upstream** in the oriui repo (happy-dom can't compute rendered color-contrast).
  3. **Browser axe over the REAL rendered `/draw`** — Playwright + `@axe-core/playwright`, script `test:a11y` in `apps/web`, **deliberately NOT in `lint:all`** (a heavy headless-browser run; a separate command). The allowlist is per-element `AxeBuilder.exclude()`, **not** a rule disable — so `color-contrast` stays active everywhere else and a real regression still fails. `apca-w3` is a **future advisory**, not a gate.
- **`.menu__section-title` contrast fixed.** The muted side-menu section headers composited only 4.01:1 on the light surface; `opacity` 0.6 → 0.7 lifts them to 5.39:1 (dark theme was already ~6:1). Removed from the `test:a11y` allowlist so the browser axe suite now **enforces** it.
- **Interim `--ori-tooltip` override — briefly removed, then RESTORED (correction, same day).** It was removed on the belief that registry `@oriui/css` 1.0.0-alpha.6 shipped the fixed bubble. **That was wrong.** The fix (self-paired neutral bubble on the anchored, out-of-flow primitive) is committed on oriui `feat/neutral-skin-tooltip-contrast` (`bd3d847`) but **not yet merged/published** — alpha-6 was cut from oriui `main` before it, so published alpha-6's tooltip is still mis-paired (`bg: var(--ori-color)` → invisible on our dark ink) **and** in-flow (crowds tight mobile clusters). This app renders the tooltip correctly only because it currently runs the oriui **branch tarball** locally; a clean install from the registry would regress. So the override stays — inert on the tarball, protective on registry — until oriui publishes the fix and this app bumps (the `IDEAS.md` item). Being **unlayered** it also beats oriui's role-color rule `.ori-tooltip:where(.ori-color_primary,…)`, but the app uses only neutral tooltips, so that's moot. ✅ **RESOLVED 2026-07-08 (alpha-7)** — oriui published the anchored self-paired bubble; the interim `--ori-tooltip` override is gone (see the top 2026-07-08 entry).

## 2026-07-08 — Hand tool, coord readout, and the a11y-tooling architecture

Two editor affordances plus a decided three-layer accessibility-tooling stack (oriui bumped again for a new skin + a tooltip fix).

- **Hand (pan) tool — a type-safe non-stroke tool.** Added to the editor as a `kind: 'pan'` member of the tool union; the editor routes pan **before** the stroke path and it has no `buildStroke`, so it structurally cannot touch the document or history (a pan can't mutate or undo anything). It brings **single-finger touch pan** — touch users previously couldn't pan at all (middle-button only) — a grab cursor, and hides the brush ring while active. Ships alongside `editor.toDocumentCoords(clientX, clientY)`, which backs a desktop cursor-coordinate readout.
- **oriui bump — a new `neutral` skin.** oriui now ships a `neutral` skin (pure neutral grays, monochrome accent — the vueinjar-era palette, now a first-class oriui skin). The app still sets its own tokens in `main.css`; adopting `data-ori-skin=neutral` to delegate the base palette to oriui is a future option (`IDEAS.md`), not done here.
- **A11y-tooling architecture — three complementary layers, decided.** Each catches what the others structurally can't:
  1. **Token-source WCAG check (colord + its a11y plugin).** justpaint's `scripts/check-contrast.mjs` and oriui's `tests/tokens.contrast.test.ts` both now use **colord** (shared, vetted — replaces the hand-rolled contrast math). This is the layer axe **cannot** do: a static token guard with no render.
  2. **Component axe in happy-dom** (oriui already has it) — structure only (roles / ARIA / names), **NOT** contrast (happy-dom doesn't render, so it can't judge color pairings).
  3. **Browser axe (Playwright + `@axe-core/playwright`)** over the **real rendered** app — the only layer that catches rendered mis-pairings like the tooltip bug. oriui already runs 40 Playwright e2e; justpaint's browser-axe over `/draw` is the **pending next unit** (`IDEAS.md`).
  `apca-w3` (WCAG-3 draft Lc) is a **future advisory** signal, not a gate.
- ✅ **RESOLVED 2026-07-08 (alpha-7).** **Tooltip pairing/collision bug fixed at the oriui root** (oriui branch `feat/neutral-skin-tooltip-contrast`, commit `bd3d847`) — **not yet published**. Until oriui publishes, justpaint carries an **interim** `--ori-tooltip` override + per-cluster `placement` (`NOTES.md`); on the version bump only the `--ori-tooltip` override was dropped (`IDEAS.md`) — the directional `placement` props **stay** (normal anchored placement now, not the old in-flow workaround).

## 2026-07-08 — Legacy shell returns: right-side menu, palette, names, backdrop (owner batch 2026-07-07)

The owner's 13-point batch restored the legacy justpaint shell on the new vector editor (oriui bumped to **1.0.0-alpha.6**). Pinned choices:

- **Menu opens from the RIGHT, non-modal** (the legacy mechanic): always-mounted 400px panel (`max-width: 100dvw` — full-screen on phones), `translateX(101%) ⇄ 0`, `border-left` primary, NO backdrop — the canvas stays interactive behind it. The toggler is a fixed top-right chip ABOVE the open panel (`z-index` 110 vs 100), icon `mdiMenu ⇄ mdiMenuOpen`. Non-modal ⇒ no focus trap / no `aria-modal` (Esc + focus-return kept).
- **Unregistered users are the `/draw` priority** (game mode prioritizes registered): menu order is title → Copy as text/image → File → Canvas → auth at the BOTTOM. "Copy as text" copies the **document JSON** (the raster data-URL was a legacy artifact); "Copy as image" copies a PNG blob.
- **Drawing `name` is drawing METADATA, not document format**: a `drawings.name` column (64-rune cap, `'new art'` default, update-keeps-on-absent) through migration/sqlc/handler/client — the document validators are untouched. Title edited in the menu header (contenteditable, signed-in only), persisted on save.
- **Legacy palette**: surfaces `#f0f2f6`/`#191919`, backgrounds `#ffffff`/`#121212`, primary `hsl(20 100% 50%)`/`hsl(20 100% 60%)` with DARK-INK on-primary (white fails AA on this orange). Colored-TEXT button variants darken to `hsl(20 100% 38%)` in light (4.64:1 — the accent/readable-text split); outlines recomputed ≥3:1 vs the new surfaces.
- **Canvas backdrop is a VIEW preference, not document state**: new docs are viewport-sized with `background: null`; behind them the editor paints theme paper (white/black) or the legacy checkerboard via `Editor.setCanvasBackdrop` (view-only Konva layer, zoom-compensated tiles, never exported). Persisted in `localStorage['jp.backdropGrid']`; default = paper. PNG export of a transparent doc stays transparent (legacy parity); the judge renders on white regardless.
- **Layers panel**: desktop = dropdown under the top-right actions island; phones = full-width bottom sheet with a scrim. Toolbar style controls collapse into an **OriPopover** on phones (variant C); numeric px width input beside the slider; shortcuts cheat-sheet is desktop-only.

## 2026-07-07 — AI Assist v1 design accepted (text drawing commands)

The first AI-in-product feature (item (a) of `IDEAS.md` "AI inside the product") is designed and accepted — full design in **`docs/ASSIST.md`**. The pinned choices:

- **Ops contract over the command seam.** The LLM emits a small discriminated union of document operations (`add_layer`, `add_stroke` restricted to `line|rect|ellipse|polygon` — freehand excluded in v1), never Konva. The Op schema joins the dual-validator contract (`packages/document` TS ⇄ `server/internal/document` Go, 1:1 parity like the Stroke contract). Edit ops (`update_stroke`/`delete_stroke`) are a v2 placeholder.
- **Go proxy module `internal/assist`, a judge-style seam.** `Assist` interface + fake impl (dev/CI default) + real impl on the official Anthropic Go SDK; `POST /api/assist/ops` (auth required). The proxy exists chiefly so `ANTHROPIC_API_KEY` stays server-side. Server validates every op against the Go validator, one retry with validator errors appended, then 422; per-user rate limit ships with the feature (public demo, paid API).
- **Anthropic structured outputs** (`output_config.format` json_schema, `additionalProperties: false`, no recursion) guarantee parseable schema-conformant JSON — the retry path handles only semantic failures. Non-streaming `Messages.New`; the client sends a compact doc summary (bboxes, freehand points elided), never the full jsonb.
- **Ghost-preview + accept UX.** Returned ops render as a ghost preview outside history; Accept maps the batch into one composite `Command` via `Editor.commit()` (whole AI action = one Ctrl+Z), Reject discards.
- **Model: `claude-opus-4-8`** by default (shape-composition quality), configurable via `ASSIST_MODEL`; `claude-haiku-4-5` documented as the budget option.

Phasing (ASSIST.md §7): A = MVP (v1 ops + preview + fake/real impls + prompt panel), B = edit ops + iterative chat, C = freehand, inpainting (needs a raster stroke type — separate decision), real judge impl on the same Anthropic plumbing.

## 2026-07-04 — UX-first: polish /draw + shell before the /play UI

The owner directed: improve the site's and `/draw`'s UI/UX **before** building the game-mode UI. The game *backend* (async duel loop + authoritative render) is done and unaffected — this re-sequences only the frontend work: the UX pass now gates the `/play` page (tracked in `ROADMAP.md` Phase 3).

**"Keep `/draw` minimal" is reinterpreted:** still *focused* — editor + save/load, no feature creep — but **polished**: proper oriui layout/spacing (the toolbar is currently cramped, no gaps/margins), the legacy **slide-in side menu** pattern returns (hidden by default, opened by a toggle button; auth + profile live inside it — the interface the owner specifically liked, see `IDEAS.md` "Salvaged from the `/legacy` app"), and canvas interaction correctness (no drawing outside the document area, immediate eraser feedback).

Also declared: the **AI-in-product** direction (see `IDEAS.md` "AI inside the product") as a planned north-star upgrade — AI features inside the product itself; priority high, sequencing vs `/play` TBD.

**Pinned choices (owner, same day):** layout = a **modern floating bottom toolbar** (tldraw/FigJam style), explicitly chosen over patching the current top toolbar. The editor shell is **shared between `/draw` and `/play`** — one design system; the game mode adds its chrome (prompt banner, timer, submit) around the same shell, UI/UX must not diverge. **All popular keyboard shortcuts** (tool hotkeys, Ctrl+Z/Y, Ctrl+0/±, Ctrl+S save) + a shortcuts cheat-sheet. The side menu holds **auth + profile** and **replaces the top SessionBar** (saved-drawings list + settings come later). Canvas bounds: **clip layers to the document rect + ignore gestures that start outside** — a stroke started inside may extend past the edge but is visually clipped (Figma-frame style). Sequencing confirmed: UX pass → `/play` UI → then AI-in-product.

## 2026-07-03 — The authoritative render worker (Phase 3)

Realizing the `render.Renderer` seam with the real thing (`feat/render-worker`): a headless Node worker that renders the vector document to the judged raster off the client.

### One shared renderer — the worker reuses the editor's `renderToStage`
The judged raster must be produced by the **same** Konva + perfect-freehand path the editor draws with (why `FREEHAND_VERSION` is pinned) — a second renderer (e.g. a Go rasterizer) would silently diverge. So the worker imports `@justpaint/editor`'s projection, not a reimplementation. To make that path headless-safe, `toKonva`/`renderToPNG` were split: a DOM-free **`renderToStage`** (container guarded by `typeof document`) is the shared core; `renderToPNG` (browser, `toBlob`) and the worker (`stage.toDataURL` under `konva/canvas-backend`) are thin output tails over it. The browser path is byte-identical (verified: `/draw` export still yields a valid PNG); the worker renders the same pixels headless (verified live).

### Stack: node-canvas + Konva 10 (`konva/canvas-backend`), viable on Windows
A feasibility spike confirmed `canvas` (node-canvas) installs from a prebuild (no node-gyp) on Windows + Node 24, and Konva 10 renders headless once `konva/canvas-backend` is imported **before** any Konva use (Konva 10 dropped the default Node backend). node-canvas is now a real repo dependency (native) — documented in NOTES; a fresh `npm install` builds/fetches it.

### The worker is esbuild-bundled (the packages aren't Node-ESM-loadable)
`@justpaint/{editor,document}` emit **extensionless** relative imports (`./ids`) — fine for Vite/the browser app, but native Node ESM refuses them. Rather than churn `.js` extensions across the frozen contract package + editor (and risk the mirror), the worker is **bundled with esbuild** (`packages/render/dist/render.mjs`), inlining the workspace packages and keeping `canvas` external (native). Isolated to one new package; zero change to document/editor source.

### Go ⇄ Node: spawn-per-render, document JSON in / base64 PNG out
`render.NodeRenderer` (`server/internal/render`) shells out (`exec.CommandContext`) to `node dist/render.mjs`, piping the document JSON on stdin and reading the PNG **base64** on stdout (base64 keeps the pipe text-safe cross-platform). Spawn-per-render (not a long-lived service or queue) is right-sized for v1 async judging (two renders/match, off the request path); a resident worker is a later optimization. The `Renderer` interface stays `Render(ctx, document.Document)`; `NodeRenderer` re-marshals the validated doc to JSON (unknown fields are irrelevant to rendering).

### `RENDER_MODE` config — stub default, node opt-in (fail-fast)
`RENDER_MODE=stub` (default) keeps the zero-dependency in-process stub so the Go server runs with no Node/canvas present (dev, CI, tests). `RENDER_MODE=node` selects the authoritative worker and **requires `RENDER_CLI`** (path to the bundle) — the server fails fast at boot if it's unset or the mode is unknown, matching the `JWT_SECRET`/`DATABASE_URL` fail-fast posture. This keeps "correct by default to run" and "authoritative when configured" both true, and makes the seam a one-env-var swap.

## 2026-07-03 — Submit, judging & the render seam (Phase 3, loop closed)

Closing the async-duel loop (`internal/game`: `POST /api/matches/{id}/submit`, out-of-band judging, `GET …/result`). The lifecycle now runs `open → drawing → judging → done` end-to-end (verified live).

### The renderer is a seam, exactly like the judge (`render.Renderer` + `StubRenderer`)
The authoritative judged raster must be rendered **off the client** from the vector document (trust boundary, `GAME.md` §6). It must also match the editor's output pixel-for-pixel — which is why `FREEHAND_VERSION` is pinned — so the real renderer has to be the **same** Konva + perfect-freehand path the editor uses, i.e. a **Node** worker (native `canvas`), not a reimplementation in Go (a Go rasterizer would silently diverge). Rather than block the loop on standing up that worker (native deps, a new deployable), we mirror the Judge seam: a Go `render.Renderer` interface with an in-process **`StubRenderer`** (deterministic 1024² PNG, ink coverage ∝ stroke count). The whole submit → render → judge → result loop runs today; the real Node worker swaps in behind `Renderer` with **no loop change** — the same guarantee we already have for `Judge`. The stub is loop-proving, not pixel-authoritative (`NOTES.md`).

### Judging is out-of-band (in-process goroutine), per API.md §8.3
Submit returns **202** immediately; the verdict is produced asynchronously. On the final submit the handler flips the match to `judging` inside the submit tx, then spawns a goroutine (own `context.Background()` + timeout) that renders both docs, calls the judge, maps the positional winner → player id, applies Elo, and commits `judging → done` in one tx. **v1 accepts the durability gap:** a crash mid-judge leaves the match stuck in `judging` (no auto-retry). A restart-time sweeper that re-drives `judging` matches lands with the real render worker (`IDEAS.md`). In-process (not a queue) is right-sized for v1 — the judging work is CPU-bound and sub-second with the stub/fake.

### Last-submit detection is serialized by a match-row lock
Two players submitting at the same instant could each count the other as "not yet in" and neither would flip to `judging` (match stuck in `drawing` with both submitted). The submit tx takes `SELECT … FOR UPDATE` on the match row (`GetMatchForUpdate`), so the two submits serialize and exactly one observes "I'm last." A duel has only two players, so lock contention is negligible.

### Submit to a match you're not in is 403, not a hidden 404
The read paths hide a foreign match as 404 (no existence leak). **Submit is the deliberate exception** (`API.md` §1, §8.3): a player submitting to a match they hold an id for but aren't rostered in is a *known-ownership violation* → **403 forbidden**. (`ErrNotPlayer` on submit → 403; the same non-membership on `GET …/result` → 404, since a reader shouldn't confirm existence.)

## 2026-07-03 — Match creation & matchmaking (Phase 3, front half)

Building the async-duel front half (`internal/game`: `POST /api/matches` create/auto-join + `GET /api/matches/{id}`). API.md §8 deliberately left the *matchmaking* mechanism open ("create an open match the next caller joins, or pair immediately"); these pin the v1 choices.

### Matchmaking = open-pool auto-join (single "play" button)
`POST /api/matches` is the one entry point for both players. In one transaction it: **(1)** auto-joins the oldest waiting `open` async match the caller isn't already in — flipping it `open → drawing`; else **(2)** returns the caller's own still-open match if one exists; else **(3)** creates a fresh `open` match with one random active prompt pinned. Rationale: the simplest complete loop that's actually playable — two users each hit "play" and get matched, no separate create-vs-join UI, no invite/lobby. The route shape (`API.md` §8) is stable if we later add invite/ranked matchmaking. Concurrency is safe via `FOR UPDATE SKIP LOCKED` on the join candidate: two simultaneous joiners each grab a *different* match (the loser skips the locked row and opens its own), so a match is never double-seated (verified live).

### No duplicate open matches from one player (step 2)
A waiting player who taps "play" again gets their **existing** open match back, not a second one (one extra `FindMyOpenMatch` query on the miss path). This dedupes the common **sequential** re-tap. It is **best-effort, not serialized**: under the default Read-Committed isolation, two *truly concurrent* creates from the same not-yet-seated user each miss the other's uncommitted row and can still open two matches. That is benign self-clutter — never a double-*seat* (the `match_players` composite PK guarantees that) — and acceptable for v1. The hard fix (a `pg_advisory_xact_lock(userID)` at the top of the create tx, or a partial unique index) is folded into the deferred rate-limit slice (`IDEAS.md`, same abuse class). *(jp-go + jp-security review, feat/game-matches.)*

### Prompt text is redacted until the match leaves `open`
The pinned prompt's `text` is withheld while `status = open` (only `id` is sent); it is revealed once the match enters `drawing`. This enforces `GAME.md` §5 fairness — a creator waiting alone must not be able to pre-draw before the opponent joins; both get the prompt at effectively the same moment (the joiner's create response is already `drawing`+text; the creator sees it on their next poll). **This overrode the `API.md` §8 example**, which mistakenly showed `text` in an `open` response — GAME.md owns reveal timing, so API.md was corrected to match.

### Opponent identity: userId + optional displayName only — never `login`
The roster query deliberately does **not** select `users.login`. `login` may be an email (identity is email-*or*-nickname, 2026-06-19 decision); exposing it to the opponent would leak PII. Players are shown `userId` + nullable `displayName`; a null name is the client's problem to label ("Player 2"), not a reason to fall back to `login`.

## 2026-06-20 — Deferred hardening (backend review)

A multi-agent adversarial review of the Go backend (29 confirmed findings) drove a batch of fixes; two items were **intentionally deferred** — not Phase-1 deliverables, unreachable or low-risk at the greenfield stage. One (duel immutability) has since landed; rate limiting remains deferred:

- **Rate limiting (`429 rate_limited`).** `web.CodeRateLimited` is reserved and `docs/API.md` advertises 429 on auth + write routes, but no limiter ships in Phase 1. Add a per-IP / per-login limiter (or enforce at the edge/reverse proxy) before any public deploy. The implemented anti-enumeration (generic `invalid_credentials` + dummy-hash timing) stands without it.
- **Duel-submission immutability (`409 conflict`).** ✅ **RESOLVED 2026-07-03** (`feat/game-submit`, caught by the jp-security lens). The defer was gated on "unreachable while `drawings` create always sets `match_id = null`" — the submit path fired that precondition by creating `match_id`-set drawings, so the guard **had** to land with it: `UpdateDrawing`/`DeleteDrawing` now carry `and match_id is null`, and the drawings service classifies the resulting no-op as `ErrDuelLocked` → **409** (vs 404 for a genuinely absent/foreign row). Without it, a player could `PUT` a better document over their submission before the opponent submitted, and judging would render the swapped doc — breaking the trust boundary + Elo fairness.

## 2026-06-19 — Phase 0 contract resolutions

Resolving the open questions the Phase 0 specs surfaced.

### Square canvas + square judge frame
The game canvas is **square 1080×1080**; the judge renders a **square 1024×1024** frame. Both duelists share this canonical square. Rationale: letterbox bars count as judged pixels and skew similarity — square↔square avoids them. The general free-draw document default stays 1920×1080 (DOCUMENT-FORMAT §2); only the **game** pins square (owned by `docs/GAME.md`).

### Game screen visibility
During a round each player sees only their OWN canvas; both canvases are revealed on the end-of-round result screen. A `docs/GAME.md` detail (informs the WS/state design, not the document schema).

### Ties are allowed
The judge may return `winner: "tie"`; `matches.winner_player_id` is therefore nullable. Tie handling for ratings (e.g. shared/half points) is a `docs/GAME.md` detail.

### Auth: login (email or nickname) + password, optional display name
Identity is a single **`login`** credential that may be an email OR a nickname, plus a password. **`display_name`** is optional. Supersedes the earlier ARCHITECTURE sketch's email-only `email citext` field. Exact rules (format validation, case-folding) owned by `docs/API.md` / Phase 1.

### Document size / DoS caps
The binding limit is **total input points**. Computed acceptable v1 values (tunable; pinned authoritatively in `docs/API.md`):

| Limit | Value | Why |
|---|---|---|
| Request body (`http.MaxBytesReader`) | 8 MB | Outer DoS guard. A max legit doc (~100k points ≈ 2.5–3 MB jsonb) sits well under it, so the semantic points cap — not the byte cap — trips first on real drawings. |
| Total input points (all strokes) | 100,000 | A frantic ~5-min duel ≈ 30k points; 100k gives generous headroom and still rasterizes sub-second at 1024². The binding semantic cap. |
| Points per single stroke | 10,000 | Bounds one pathological stroke. |
| Total strokes | 5,000 | The points cap binds first; this is a sanity ceiling. |
| Layers | 64 | Far above any hand-editor need. |

## 2026-06-19 — Foundational decisions

### Game is the north star; editor is supporting
The centerpiece is an AI-judged drawing duel, not the paint editor. The standalone editor (`/draw`) is a supporting mode, kept minimal. Rationale: a generic paint app is a commodity portfolio project; the AI-judged game is the memorable differentiator. Don't build two products.

### Two modes in one site (not only-game, not separate sites)
`apps/web` serves `/draw` (free) + `/play` (game). The editor exists for the game anyway, so `/draw` is near-free and useful (dev surface, home for save/load, fallback demo). Separate sites = premature ops/focus split for a solo project; the reusability signal already comes from the `editor` package boundary. Reversible later if the game earns its own brand.

### Use libraries for canvas; don't hand-write an engine
Rendering on **Konva** + **perfect-freehand**; we own only the document model/serialization and a thin editor wrapper. Rationale: a custom render engine reinvents Konva/Fabric, doesn't advance the learn-Go goal, and isn't the differentiator. Own the boundaries, rent the rendering. **Konva over Fabric** — explicit layers, speed, JSON serialization, `vue-konva`, clean PNG export for the judge.

### Backend rewritten in Go as a modular monolith
Replace NestJS with one Go service (auth + drawings + game + WS hub), one Postgres. Stack: net/http + pgx + sqlc + goose + golang-jwt + bcrypt + slog + coder/websocket. Rationale: learn-Go is an explicit goal; a monolith is right-sized for solo; microservices would be a portfolio anti-pattern. Don't refactor the old NestJS — replace it.

### Vector document persisted as jsonb
Drawings stored as a structured vector document (`Document/Layer/Stroke`), not bytea PNG. Enables small storage, clean export, real layers, replay, and a clean contract for the judge. The schema is independent of Konva's internal JSON to avoid vendor lock.

### The ML judge is external (built by a collaborator)
A friend builds the ML judge (his own portfolio piece). We define the `Judge` interface (prompt + 2 images → `{scoreA, scoreB, winner, reason}`) and ship a fake impl; he integrates over HTTP against the live contract. Never block on the ML.

### Greenfield
No production data to preserve; free to redesign schema/format. (If this ever changes, revisit migration strategy.)

### Component library: oriui (replacing vueinjar)
Frontend uses the owner's own component library **oriui** (alpha, on npm) instead of `vueinjar`. Dogfooding. Current `vueinjar` surface to migrate: `VButton`, `VCard`, `VIcon`, `VAvatar` across ~12 components.
