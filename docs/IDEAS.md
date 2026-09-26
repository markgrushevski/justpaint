# Ideas & backlog (non-blocking)

> Parking lot for improvements surfaced mid-build that are **deliberately deferred** — not bugs, not Phase-1 blockers. Each item has a one-line rationale + a rough *when*. Promote an item into `ROADMAP.md` when its phase goes active. Hard, already-made trade-offs live in `DECISIONS.md`; this is the softer "good ideas, later" list.

## AI inside the product (north-star upgrade)
AI features belong **inside** the product itself, not just as a dev tool — closes the main product gap and canvas + LLM integration is a striking demo. Text drawing commands have shipped (`docs/ASSIST.md`); these two are still open:

- **AI inpainting / draw-completion** — via an external image API; the Node render worker already produces server-side rasters to feed it. Results could come back as raster layers (needs a version-safe additive document field, format §9) or as vector strokes. *When:* needs an image API and that document extension.
- **Canvas co-author assistant** — an agent that draws alongside the user / critiques / suggests, over the same command seam as text commands. *When:* needs a UX that isn't a one-shot prompt box.

## AI assist (`internal/assist`)
- **Rate-limit buckets are never evicted** — `RateLimiter` (`server/internal/assist/ratelimit.go`) keys per-user token buckets in an unbounded map with no TTL sweep. Lower stakes since the daily AI-call ledger (`internal/aibudget`) also bounds assist and survives restarts, but the map still grows unbounded. *When:* add TTL eviction once assist traffic actually grows.
- **Surface the AI-assist panel in `/play` too** — the prompt → ghost-preview → accept flow (`packages/editor`'s `previewOps`/`acceptOps`/`rejectOps`) is only mounted in `/draw`. *When:* after the core `/play` loop stabilizes; needs a `/play`-appropriate UI treatment (the round timer/turn structure differs from free `/draw`).

## AI budget (`internal/aibudget`)
- **A per-provider global ceiling** — `AI_DAILY_GLOBAL` is one number applied to every provider, so Google's free tier and the external judge service (`collaborator`, `JUDGE_MODE=http`) would share a figure that can only be right for one of them. The same variable can absorb `google=200,collaborator=50` when it's needed, with no second variable. *When:* the day a second provider is actually enforced — today only `google:<model>` runs.
- **A `Remaining(kind, userID)` port** so the UI can say "1 guess left" instead of only refusing at zero — the ledger already answers the question (`CountUserKindCallsInWindow`). *When:* next touch of the guess or practice UI.
- **Rename `internal/judge` → `internal/vision`** — the package holds three seams and two of them (`Critic`, `Guesser`) are ours, not the external judge's. `docs/JUDGE.md` keeps owning the contract regardless of the Go package name. *When:* before a fourth seam makes the mismatch worse.

## Auth / identity
- **Login charset/format validation** — `login` is validated by length only (3–254 chars). Add format rules: if it contains `@`, validate as an email; otherwise restrict a nickname to `[a-zA-Z0-9_.-]` (no spaces, no emoji). *When:* cheap — fold into the next auth touch.
- **Optional separate `email` column** — split the sign-in handle (`login`) from a verified contact `email`, unlocking password reset, email verification, notifications. *When:* only when an email flow is actually built; the single `login` (email-or-nickname, citext) stands until then.
- **Auth surface upgrades** — candidates, roughly by impact: guest/anonymous quick-play (jump into a duel on an ephemeral account, then optionally claim it by registering); a global, mode-agnostic sign-in affordance; resume-after-auth (land back on the pending match, not a generic redirect); OAuth (GitHub/Google) and/or passkeys; form-feel polish (inline validation, password-strength meter, "remember me"). *When:* guest-play + resume-after-auth first (cheapest, highest funnel value); OAuth/passkeys once an email/identity flow exists.

## Observability (server)
- **Request-id correlation** — generate (or read `X-Request-Id`), attach to the slog context and echo in the response header, so every log line of one request correlates. *When:* next backend hardening pass; cheap, high value.
- **Prometheus `/metrics`** — a metrics middleware (`http_requests_total{method,route,status}`, a latency histogram, in-flight gauge), `pgxpool.Stat()` gauges, Go runtime collectors. *When:* when dashboards are wanted.
- **Loki + Grafana (logs)** — the JSON-to-stdout slog is already Loki-friendly; shipping it is an ops concern, not code. *When:* deploy time.
- **OpenTelemetry tracing** — traces → Tempo/Jaeger, viewable in Grafana. *When:* later; overkill for v1.

## Drawings / API
- **Server-generated `thumbnail_url` only** — store only a server-side object-storage URL, never a client-supplied one (avoids stored-SSRF/XSS via a poisoned URL). *When:* when thumbnails are implemented.
- **One open match per user (concurrency hardening)** — `FindMyOpenMatch` dedupe (`DECISIONS.md` 2026-07-03) is best-effort under Read Committed: a truly concurrent double-tap can still open two `open` matches (never a double-seat — the composite PK guards that). Enforce with `pg_advisory_xact_lock(hashtext(userID))` or a partial unique index. *When:* next time matchmaking is touched.

## Game — judging & render
- **Concede** — let a player leave a live match early (it resolves to `abandoned`). The deadline already resolves stranded rounds, so this is UX, not correctness.

## Realtime (`internal/ws`)
- **Multi-instance hub via Postgres `LISTEN/NOTIFY` or Redis, behind the `Publisher` seam** — the in-process, single-goroutine hub keeps rooms/connections in one process's memory, so horizontal scaling would strand half of any room on the wrong instance. `game.Publisher` is the deliberate extraction point — swap the in-process fan-out for a pub/sub-backed one with no `internal/game` change. *When:* only if connection count or availability needs actually force horizontal scaling.

## Frontend / build
- **CSRF posture is SameSite=Lax only** — state-changing endpoints lean on `jp_session` being `SameSite=Lax` rather than a CSRF token; every mutation is same-origin and no CORS header is sent. *When:* revisit (token / double-submit) if the threat model widens — a second origin, a public API, or cross-site embedding.
- **Custom canvas guides** — user-placed guide lines on `/draw`: unlimited count, horizontal and vertical, draggable, view-only (never exported/judged). Later: snap-to-guide for shape tools. *When:* a `/draw` power-user pass after `/play`.
- **Document background-color control** — the document's `background` is `null` by default; there's no UI to set a real background color. Needs an undoable `setBackground` editor command + a color well in the menu. *When:* when someone asks for exports with a baked background.
- **Theme picker as `OriMenu`** — the theme chip blind-cycles auto→light→dark; a menu with the three states would make them discoverable (blocked until Firefox ships CSS anchor positioning, which `OriMenu` needs). *When:* shell polish.
- **Extend browser test coverage past `/draw`** — the a11y suite (`test:a11y`) and the layout/chrome-overlap suite (`test:layout`) both only cover `/draw`; extend to `/play` and `/practice`. Both are local-only gates today (they need a dev server up) — finding a way to run them in CI is a separate open item.
- **APCA (`apca-w3`) as a supplementary advisory** — an additional contrast signal in the WCAG-3 direction, better-behaved for the brand orange than the WCAG-2 ratio. Advisory only, not a gate. *When:* alongside the next a11y touch.
- **Adopt oriui `data-ori-skin=neutral`** — delegate the base palette to oriui's `neutral` skin and drop the `main.css` palette override, keeping only the desk/backdrop token. *When:* a shell-token cleanup pass.
- **`/draw` contrast triage (pre-existing, non-blocking)** — two oriui tokens (tonal-button text, selected-tab text) sit under the 3:1 non-text bar against their surfaces; both are upstream (`@oriui/css`, see `docs/ISSUES-OUTER.md`). The brand-orange wordmark on `/draw` is 2.85:1 as large text (a brand-token decision, desktop-only). The floating `/draw` chrome also isn't wrapped in a landmark region. *When:* next a11y touch.

## Saved drawings
- **A browser for saved drawings** — a list with thumbnail and date, instead of picking by name. Needs thumbnails, which need object storage or a client-rendered preview.

## Design & UX ideas (from similar tools & games)
Researched from drawing editors (Excalidraw, tldraw, Figma/FigJam, Photopea) and drawing-duel games (Skribbl.io, Gartic Phone, Draw Battle, Jackbox Drawful). **Recorded only — not building now.** Grouped by surface; the game items serve the north star (`/play`).

**Editor & canvas (`/draw`, `packages/editor`)**
- **Bottom-docked / floating toolbar** — thumb-reachable, doesn't eat the canvas; adapts to a compact bar on mobile (tldraw, FigJam).
- **Zoom / pan / fit gestures** — pinch-zoom, two-finger pan, wheel+shift; `Ctrl+0` fit-all, `Ctrl+1` 100%.
- **Compact color control** — a preset swatch grid (~18 colors) + recent-colors history + a "more" button to the full picker; eyedropper (Alt-click) (Photopea, Sketchful).
- **Brush-size preview** — a live circle under the cursor / next to the tool showing size + opacity before drawing (Procreate, Photoshop).
- **Layer thumbnails** — mini previews per layer in the panel (Figma, Photopea) — pairs with the server `thumbnail_url` idea.
- **Visual undo-history** — a hoverable list of recent actions; click to jump back (Photopea History, Krita).
- **Shortcuts cheat-sheet** — a `?`/`Cmd+?` modal listing keys (Excalidraw, tldraw). Extra tools: fill-bucket, more shapes.

**Game — lobby & match (`/play`)**
- **Minimal lobby** — big prompt text, ready-up toggle (auto-start when all ready), live player-avatar list with ready dots; host settings via sliders (rounds, seconds/round, public/private) (Skribbl, Gartic Phone).
- **Centered prompt reveal** — the prompt appears center-screen ~3s at round start, then each player draws their own canvas (Gartic Phone, Drawful).
- **Round timer bar** — top progress bar, green→orange→red under 10s, with an alert cue (Skribbl).
- **Split view for spectators** — both canvases side-by-side for the audience; each player sees only their own during the round (Draw Battle).

**Game — result & rating (`/play`)**
- **Animated result card** — both drawings side-by-side, the ML similarity as a 0–100% bar, the judge's one-line reason, an arrow animating to the winner (Game UI Database, LoL victory screen). A `?` tooltip expands the judge's rationale (transparency).
- **Score pop + leaderboard** — floating "+15" juice on award; a leaderboard table (nick / avatar / score / Δ) highlighting the current player; a match-summary screen with Rematch / Share (Skribbl, Jackbox).
- **One-click share** — export the round (both drawings + result) as a PNG/GIF for social (Gartic Phone album, ShareX).

**Cross-cutting — theme, responsive, feel**
- **Mobile portrait layout** — canvas ~70%, toolbar bottom, panels collapse to icon-tabs; primary actions in the bottom third for one-handed reach.
- **Game-feel polish** — smooth 200–400ms easings, toast notifications for match events, skeleton loaders while the judge scores, optional audio cues.
- **Playful brand type** — a hand-drawn display font (Excalidraw's Virgil / Caveat) for lobby & result headings to set a fun tone; keep a clean system/`Nunito` body.

*Priority for the game MVP:* bottom toolbar · compact color picker · animated result card · round timer · match summary · mobile layout. Editor polish (layer thumbnails, brush preview, zoom/pan) and juice (score pop, audio, replay) come after the core loop.
