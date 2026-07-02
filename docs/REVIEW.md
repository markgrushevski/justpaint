# Review checklist

The bar a change must clear before it's "done". The **mechanical** layer is commands — don't
hand-check what tooling asserts (there is no CI yet, so run these locally; a GitHub Actions gate is
an [IDEAS.md](IDEAS.md) item):

```sh
# TypeScript workspaces (repo root)
npm run types        # vue-tsc / tsc --noEmit across packages/* + apps/*
npm run test         # Vitest across workspaces
npm run build        # package dist/ + vite build

# Go service (in server/)
gofmt -l .           # must print nothing
go vet ./...
go test ./...
go build ./...
```

This file is the **judgment** layer on top: the criteria tooling can't assert. Companion to
[`CLAUDE.md`](../CLAUDE.md) (conventions / how), [DECISIONS.md](DECISIONS.md) (rationale / why),
and [NOTES.md](NOTES.md) (gotchas). A criterion deliberately not met is fine **only if** the
exception is recorded in [DECISIONS.md](DECISIONS.md) — otherwise it's a finding.

## How to apply it

- **Default** — self-review the diff against the relevant sections before committing.
- **Orchestrated** — the read-only lenses in `.claude/agents/` each own a section below:
  `jp-contract-parity`, `jp-security`, `jp-go`, `jp-frontend`, `jp-scope-guard`, `jp-docs-reviewer`.
  They report findings against this bar; the orchestrator integrates and records.

## Contract fidelity — the keystone

The vector document ([DOCUMENT-FORMAT.md](DOCUMENT-FORMAT.md)) lives in **two validators** that must
never drift — `packages/document` (TS) and `server/internal/document` (Go):

- [ ] Every invariant exists on **both** sides — known version (`== 1`); `1 ≤ width,height ≤ 8192`;
      lowercase hex color regex `^#([0-9a-f]{6}|[0-9a-f]{8})$`; composite ∈ {source-over,
      destination-out}; opacity/pressure ∈ [0,1]; NaN/Infinity rejected; sizes/tapers ≥ 0;
      rect/ellipse positive dims; `strokeWidth > 0` when a stroke channel is present; point arity
      (freehand 3-tuple ≥ 1 / line 2-tuple ≥ 2 / polygon 2-tuple ≥ 3); id 1–64 chars, **unique
      across the single layers+strokes namespace**.
- [ ] DoS caps identical everywhere: 8 MB body / 100k total points / 10k per stroke / 5k strokes /
      64 layers ([API.md](API.md) §caps is authoritative).
- [ ] The TS `validate` test table and the Go validator tests stay **mirrored case-for-case**.
- [ ] Discriminated-union `Stroke` decode matches (Go `UnmarshalJSON` ↔ TS parse); a new stroke type
      is added in all three Go sites (struct+const, `unmarshalStroke`, `checkStroke`) and the TS
      mirror, or not at all.
- [ ] Render pins intact: `FREEHAND_VERSION` equals the **resolved installed** perfect-freehand
      version (currently 1.2.3, not the range floor `^1.2.0`); `computeFitTransform` (contain) and
      the `toFreehandOptions` constants unchanged — or the change is a recorded decision.
- [ ] Write-precision rounding (2dp geometry / 3dp pressure) happens **only** in
      `serializeDocument`, never in the model or a tool.
- [ ] The document payload stays **lax-decoded** (unknown fields tolerated for forward-compat); auth
      bodies stay strict-decoded. Don't unify them.
- [ ] Any format change is additive and version-safe (DOCUMENT-FORMAT §9), and the spec is updated
      in the same change (the doc wins on drift).

## Security & trust boundary

- [ ] Every user-owned row is fetched/mutated with `owner_id` (or match membership) in the WHERE —
      a foreign/missing row answers **404 not_found**, never 403 (no IDOR, no existence oracle). The
      one legitimate 403 (submitting to a match you're not in) is not confused with the 404 rule.
- [ ] `jp_session` stays HttpOnly + SameSite=Lax + Path=/ (+ Secure when `ENV != dev`); no token in
      localStorage, a body, a header, or a JS-readable cookie.
- [ ] JWT stays HS256 **alg-pinned** with a **mandatory** secret (fail-fast at startup, no empty
      fallback); identity comes from the `sub` claim, never from the request body.
- [ ] Anti-enumeration preserved: unknown-login and wrong-password both return one generic
      `invalid_credentials`, with the dummy-hash bcrypt compare on unknown login (no code / message /
      status / timing tell). Register may reveal existence (409 `conflict`) — that's intended.
- [ ] A document-bearing route wraps the body in `http.MaxBytesReader` (8 MB) **before** parsing;
      caps are checked before large allocations.
- [ ] Client-supplied rasters/URLs are **advisory only** — the judged/persisted artifact is rendered
      server-side from the vector document; a submitted duel canvas is enforced to the match size and
      locked immutable after submit. Never store a client-supplied URL.
- [ ] The error envelope leaks no SQL / stack / internal detail and no `password_hash`; new protected
      routes sit behind `RequireAuth`, not ad-hoc checks (logout is the deliberate exception).
- [ ] Known deferrals (rate limiting `429`; duel 409 immutability — DECISIONS 2026-06-20) stay
      deliberate; don't silently half-ship them.

## Go backend

- [ ] Modules talk through narrow interfaces — the **judge stays a seam** (game depends on a
      `Judge` interface + a drawings read port, never on "judge is HTTP" or "drawings are jsonb");
      infra stays in `internal/platform`.
- [ ] All SQL goes through **sqlc** (parameterized); jsonb is bound as `json.RawMessage` and stays
      opaque to SQL; a field you need to query is promoted to a column, not left in jsonb; migrations
      are additive and goose-managed.
- [ ] Errors are wrapped with context (`%w`), never ignored; `context.Context` is threaded to DB/HTTP
      calls; no panic used as control flow; graceful shutdown stays wired.
- [ ] New validator/handler paths get table-driven tests (only `internal/document` has tests today —
      grow coverage where you touch).
- [ ] No inline Go teaching in review notes — the owner opted out; report findings plainly.

## Frontend & packages

- [ ] Dependency direction holds: `packages/document` imports **nothing** internal (pure contract);
      `packages/editor` imports only document + Konva + perfect-freehand (never Vue/router/API);
      app concerns stay in `apps/web`.
- [ ] Tools stay **pure**: `buildStroke(ctx, gesture) → Stroke | null` — no side effects, no Konva.
- [ ] Konva lifecycle: every created `Stage` is `destroy()`ed (module-global registry — see NOTES);
      editors are torn down in `onBeforeUnmount`. Coords come from
      `stage.getRelativePointerPosition()`, never `pageX - offsetLeft`.
- [ ] Documents are validated at trust edges (`parseDocument` on anything from the network or a user
      file); `loadDocument` assumes already-valid input.
- [ ] No `any` at a package boundary; server data flows through TanStack Query + the single typed
      `fetch` client (typed `ApiError`, including the `network` code); the api layer stays store-free
      (no api⇄store cycle). Don't reintroduce the legacy broken-axios pattern outside `/legacy`.
- [ ] Konva stays the renderer — no hand-rolled render engine creeping back in.

## Design & responsive (oriui)

Owned by the `jp-design-reviewer` lens. The UI is built on the **oriui** design system.

- [ ] Colors/sizes read **resolved oriui aliases** (`--ori-color`, `--ori-color-surface`,
      `--ori-color-outline`, `--ori-size-*`), never hardcoded hex/px. Text uses `--ori-color-on-*`
      (never `--ori-color-primary` as body text). The justpaint **orange brand** is set once via the
      `*-light`/`*-dark` sources on `:root`; both themes hold.
- [ ] Don't reinvent oriui: `OriButton`/`OriInput`/`OriField`/`OriSlider`/`OriCheckbox`/`OriCard`/
      `OriStack` over hand-rolled buttons/inputs/ranges/panels/flex.
- [ ] Responsive at oriui breakpoints (`--ori-size-screen_*`: 600 / 840 / 1200 / 1600 / 1920 / 2560)
      across mobile (~375) / tablet (~768) / desktop: the body never scrolls horizontally; the canvas
      scrolls in its own container (a real fit scales the Konva **stage**, never CSS-transforms the
      `<canvas>` — NOTES); fixed side panels (the layers panel) reflow to a drawer/stack on narrow
      screens; touch targets are adequate.
- [ ] Contrast meets WCAG AA in both themes; focus is always visible (`:focus-visible`).

## Scope

- [ ] `/draw` stays editor + save/load — a feature that serves only free-draw and not the game needs
      a decision first (the two-products trap).
- [ ] The judge remains interface + fake; nothing blocks on the collaborator's ML; the positional
      `winner` (`"A"|"B"|"tie"`) → player-id mapping stays inside the game module.
- [ ] Roadmap order respected (don't start the game loop before the editor is real; async before
      live WS; no Phase-4 stretch before the core loop). No old red flag reintroduced.

## Docs

- [ ] [ROADMAP.md](ROADMAP.md) status updated when a deliverable lands or a phase flips.
- [ ] A single-owner fact (cookie flags, the caps, the winner/tie representation, the 1080²/1024²
      sizes) is **referenced**, not restated with a second (drift-prone) home.
- [ ] New decisions → [DECISIONS.md](DECISIONS.md); new gotchas → [NOTES.md](NOTES.md); deferred
      ideas → [IDEAS.md](IDEAS.md).

## Sign-off

A change is done when the mechanical gate is green and every applicable box above is checked — or the
gap is a recorded decision, not an oversight.
