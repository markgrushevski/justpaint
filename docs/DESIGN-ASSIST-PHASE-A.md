# AI Assist — Phase A implementation brief

> Grounded implementation brief for AI Assist Phase A — refines the accepted design in
> `docs/ASSIST.md` against the current code. Fake-first (no Anthropic API key; real impl
> scaffolded only). Status: planned 2026-07-12.
>
> `docs/ASSIST.md` still owns the canonical Op contract; this brief owns the build plan +
> the three resolutions below. Where the two disagree, this brief wins until ASSIST.md is
> amended to match (same rule ASSIST.md itself states for `DOCUMENT-FORMAT.md`).

## 1. Three locked resolutions

ASSIST.md was accepted before a close read against the current contract/API surface. These
three correct it; everything else in ASSIST.md §1–§8 stands.

| # | Claim (ASSIST.md) | Resolution |
|---|---|---|
| 1 | §3.3 returns `422` on retry-exhaustion. | `422` is forbidden — `docs/API.md:68` reserves it unused in v1 ("We prefer `400` over `422` for doc-invalid, for one consistent client path"). Retry-exhaustion returns **`400 validation_failed`** instead, carrying the last validator error in `message`. The rate-limit path (§3.4) reuses the already-reserved **`CodeRateLimited`** (`web.go:18`) for **`429`** — no new error code. |
| 2 | §2's `add_layer` has no `id` field, yet §2's Op-reference rule requires `add_stroke.layer_id` to resolve batch-created layers — a contradiction: nothing to reference. | `add_layer` gets an **LLM-assigned `id`**: `{ kind: "add_layer", id, name }`. It is validated in the **same single id-namespace** as document layers/strokes (`validate.ts:258` / `validate.go:47`) — a batch id colliding with an existing doc id, or a duplicate within the batch, fails validation exactly like a duplicate stroke id today. `add_stroke.layer_id` resolves against **(doc-summary layer ids) ∪ (earlier-in-batch `add_layer` ids)**, strictly in array order — a forward or dangling reference is a validation failure, not a lookup at apply time. |
| 3 | §4's `DocSummary` references a `StrokeStyle` type that doesn't exist and a per-stroke `bbox` that nothing computes (the document format's `bbox` is optional/advisory and never populated by the editor). | Phase-A `DocSummary` is **minimal**: `{ canvas: { width, height }, layers: [{ id, name, strokeCount }] }`. No `recent_strokes`, no per-stroke `bbox`/`style` — deferred to whenever a real prompt-composition need justifies the token cost. `FakeAssist` ignores the summary entirely (canned ops, deterministic). |

**Smaller corrections (fold into the same change):**
- `freehand` is excluded from `add_stroke` on **both** validators explicitly — the existing `Stroke`/`checkStroke` validators accept `freehand` (`types.ts:25`, `document.go:10-16` list it), so the Op validator needs its own narrower stroke-type allowlist, not a reuse of the full one.
- All Op/DocSummary/request/response DTOs are **camelCase** — the whole live API is (`apps/web/src/core/api/drawings.ts:17-36`, `matches.ts`); ASSIST.md's `layer_id`/`doc_summary`/`target_layer_id`/`stroke_count` snake_case is a drafting artifact, not intentional.
- The config mode-switch copies **`internal/render`**'s pattern (`RENDER_MODE`), not the judge's (the judge is hardwired to `FakeJudge`, `main.go:76`) — add `ASSIST_MODE` (default `fake`) + `ANTHROPIC_API_KEY` + `ASSIST_MODEL`, fail-fast at boot when `ASSIST_MODE=anthropic` without a key (mirrors `config.go:93-108`'s `RENDER_CLI` fail-fast).
- Add `maxOpsPerBatch` (~64) to **both** validators (`constants.ts:17-25` `LIMITS`, `validate.go:12-21` caps consts) — nothing in ASSIST.md bounds batch size today.
- Do **not** add the `anthropic-sdk-go` module to `go.mod` in Phase A — `ASSIST_MODE=anthropic` is scaffolded (config plumbing + a stub that fails fast with "not implemented") but not wired to a live SDK call; fake-first per the header.
- The assist endpoint only ever has the **doc summary**, not the full document, so it can only enforce **per-stroke** and **per-batch** caps (point count, batch size). Whole-document caps (`maxLayers`, `maxStrokes`, `maxTotalPoints`) still fire where they always have — at the drawings/game save write-edge, when the accepted batch is eventually persisted as part of a real save.

## 2. Per-domain build plan

Contract is the keystone — land it first; Editor/Server/Web build against it in parallel.

### 2.1 Contract (`packages/document` + `server/internal/document`)

TS side:
- **`types.ts:167-173`** (the `Stroke` union) — add the `Op` union directly below it: `Op = { kind: "add_layer"; id: string; name: string } | { kind: "add_stroke"; layerId: string; stroke: Stroke }`, constrained to the non-freehand subset.
- **`types.ts:25`** (`StrokeType`) — the Op-eligible stroke subset is `"line" | "rect" | "ellipse" | "polygon"` (freehand excluded per §1 smaller-corrections); define it as its own exported type, not a re-narrowing hack on `StrokeType`.
- **`validate.ts:184-209`** (`validateStroke`, private) — reuse verbatim for the `stroke` field inside an `add_stroke` op; do not fork it.
- **`validate.ts:59-65`** (`checkId`) — reuse for both `add_layer.id` and the ids `validateStroke` already checks; the op validator seeds the **same `Set<string>`** it's given, never a fresh one (see the single-namespace note below).
- **`validate.ts:235-278`** (`validateDocument`) — the walk/cap-accumulation pattern (`ids` Set, running `totalStrokes`/`totalPoints`, layer/stroke `path` prefixes) is the template for the new `validateOpBatch(summary: DocSummary, ops: unknown): Op[]`.
- **`constants.ts:17-25`** (`LIMITS`) — add `maxOpsPerBatch: 64`.
- **`index.ts:32-48`** (the barrel) — export `Op`, `DocSummary`, `validateOpBatch`, `OpValidationError` (or reuse `DocumentValidationError`) alongside the existing exports.
- **`test/validate.test.ts:72-163`** (`docWith` helper + the `valid`/`invalid` table + the path-reporting test) — copy this shape wholesale into a new `test/ops.test.ts`; same test names on both sides (parity discipline, §3).

Go side (mirror 1:1):
- **`document.go:89-102`** (`Stroke` sealed-interface + `StrokeBase`) — mirror the shape for `Op` as a small sealed interface (`AddLayerOp`, `AddStrokeOp`), same `base()`-unexported pattern if useful, or a plain discriminated struct with a `Kind` field — pick whichever `parse.go`'s discriminated decode (next bullet) makes cleanest.
- **`parse.go:25-79`** (`Layer.UnmarshalJSON` + `unmarshalStroke`) — copy the raw-`json.RawMessage` + `type`-discriminant dispatch pattern for decoding an `[]Op` batch.
- **`parse.go:132`** (`ParseAndValidate`) — the entry-point shape (`Parse` → `requiredKeys` → `Validate`) is the template for a new `ParseAndValidateOpBatch(data []byte, summary DocSummary) ([]Op, error)` in a new `document` (or `assist`-local) file.
- **`parse.go:154-201`** (`requiredKeys`) — mirror the raw-bytes presence guard; Go zero-fills an absent field (absent `add_layer.name` → `""`, not an error) so the same presence check is required for ops (see §3 gotchas).
- **`validate.go:28-80`** (`Validate`) + **`validate.go:84-107`** (`checkStroke`) — reuse `checkStroke` verbatim for the `stroke` inside `add_stroke`; the op-batch validator's top-level loop mirrors `Validate`'s cap-accumulation shape.
- **`validate.go:255-264`** (`checkID`) — reuse for `add_layer.id`; **seed it from the summary's layer ids**, not an empty map (single-namespace note below).
- **`validate.go:12-21`** (caps consts) — add `MaxOpsPerBatch = 64`.
- **`validate_test.go:31-97`** (`docWith` + `TestParseAndValidate`'s table + subtest loop) — copy the shape into a new `ops_test.go`, **identical case names** to `ops.test.ts` (contract-parity test asserts this).

**Single id-namespace, seeded from the summary.** `validate.ts:258` and `validate.go:47` both build one `ids` Set/map spanning layers + strokes for a *whole document*. The op validator has no whole document — only the summary. It must **seed** that Set/map from `summary.layers[].id` before validating `add_layer`/`add_stroke` ids against it, so a batch id colliding with an existing doc-layer id is caught (not just intra-batch duplicates).

### 2.2 Editor (`packages/editor`)

- **`history.ts:21-28`** (`Command` interface) — the shape every op maps to.
- **`history.ts:45-58`** (`addStrokeCommand`) — reuse directly for `add_stroke` ops (keyed by the resolved `layerId`).
- **`history.ts:61-73`** (`addLayerCommand`) — reuse for `add_layer`, **but** its `apply` clamps `index` against `doc.layers.length` *at apply time*; a batch with several `add_layer` ops needs a **running top-of-stack index** fed in at construction (increment a local counter per `add_layer` processed), or the second new layer clamps to the same index as the first and they collide in z-order.
- **`history.ts:158-201`** (the `History` class, `execute` = one undo-stack entry) — add a `compositeCommand(cmds: Command[]): Command` here: `apply` runs `cmds` in order, `invert` runs them in **reverse** (an `add_layer` then an `add_stroke` on it must invert stroke-then-layer, or the invert removes a layer that still holds a stroke reference). One `history.execute(doc, compositeCommand(...))` = one undo entry for the whole accepted batch.
- **`editor.ts:115`** (`private stage: Konva.Stage`) — no public accessor exists; the ghost preview needs its own layer on this stage, so the new methods below must live as `Editor` methods (inside the class), not free functions reaching in from outside.
- **`editor.ts:476`** (`private commit`) — `acceptOps` calls `history.execute` + this method's `afterMutation` shape directly (or calls `commit` itself if made to accept a pre-built `Command`), so accept goes through the exact same re-project + notify path as every other mutation.
- **`editor.ts:571-598`** (`renderPreview`) — reuse only the **idiom** at `:583-596` (build a throwaway one-layer `toKonva` projection, move its children onto a group, destroy the throwaway stage) for the ghost's rendering — not the field or the method itself, which is gesture-preview-specific.
- **`editor.ts:531-556`** (`rerender` — destroys and rebuilds the whole stage) — **`:549`** is where the cursor overlay remounts after rebuild (`if (this.cursorColor != null) { this.mountCursorOverlay(); ... }`); the ghost layer must remount here the same way or it silently vanishes on the next commit/undo/redo/loadDocument.
- **`editor.ts:349-353`** (inside `destroy()`) — the cursor/backdrop/preview fields are nulled here on teardown; add the ghost field to this list.
- **`editor.ts:739-767`** (`mountCursorOverlay` + `syncCursorRing`) — the "non-listening top Konva layer, rebuilt with the stage" pattern to mirror for the ghost overlay (reduced opacity + accent outline per ASSIST.md §5).
- **`editor.ts:517-528`** (`applyView`) — the one place the stage transform is set; mounting the ghost as a normal stage layer means it inherits pan/zoom for free, no extra transform math needed.
- **`index.ts:1-22`** (the barrel) — does **not** export `Command`/`History`/`*Command` today; keep it that way. The op→command mapping and the ghost implementation live entirely inside `packages/editor`, exposed only as three new `Editor` methods: **`previewOps(ops: Op[]): void`**, **`acceptOps(): void`**, **`rejectOps(): void`**. `apps/web` never imports Konva or sees a `Command`.
- **`konva.ts:167-175`** (`toLayer`) — the ghost's clip must match this doc-rect clip (Figma-frame style) so a ghost stroke past the canvas edge previews identically to a committed one.
- **`konva.ts:201-210`** (`toKonva`) — same throwaway-stage idiom referenced above.

### 2.3 Server (`server/internal/assist`, new package)

- **Config switch** — copy the `RENDER_MODE` template, not the judge's (judge is hardwired, `main.go:76`): `render.go:22` (the `Renderer` interface shape) → an `Assist` interface; `stub.go:24` (`StubRenderer`) → `FakeAssist`; `node.go:21` (`NodeRenderer`) → a scaffolded `AnthropicAssist` (constructor + fail-fast validation only, no live SDK call — §1). `config.go:40-43,61,93-108` is the exact mode-switch + fail-fast shape to copy for `ASSIST_MODE`/`ANTHROPIC_API_KEY`/`ASSIST_MODEL`; `main.go:68-77` is the composition-root wiring to mirror (`if cfg.AssistMode == ... { assist = ... } else { assist = NewFakeAssist() }`).
- **Interface + fake shape** — reuse the judge's self-validating-result idiom: `judge.go:28` (`Judge` interface, one method), `judge.go:52` (`ErrInvalidResult`), `judge.go:57` (`Result.Validate()` — the seam validates its own output before the caller trusts it); `fake.go:24` (`FakeJudge` struct — deterministic, in-process, ignores real inputs it doesn't need). `Assist.GenerateOps(ctx, req) (Result, error)` where `Result` self-validates (op-batch shape + document invariants) before the handler ever sees it, matching ASSIST.md §3.3's "validate before returning."
- **Handler pattern** — `game/handler.go:38-44` (`Routes` + the `protect` middleware parameter) for wiring `POST /api/assist/ops`; `:147` (`auth.UserID(r.Context())`) for the auth read; `:150-154` (`web.DecodeJSON` — strict, `DisallowUnknownFields`) for decoding `{prompt, docSummary, targetLayerId?}` (small fixed shape, strict decode per `docs/API.md §1`).
- **Error codes** — `web.go:10-20` is the closed code set; `:18` is the already-reserved `CodeRateLimited` (§1 resolution 1 reuses it for 429, no new code needed); `:41-53` is `Error`/`DecodeJSON` to call directly — do not add a `422` path anywhere.
- **Auth** — `auth/middleware.go:20-24` (`UserID`) / `:36-52` (`RequireAuth`) — the route requires a session exactly like every other write route (ASSIST.md §3.1).
- **Fail-fast posture** — `config.go:74-83` (missing-required-env accumulation) + `:93-108` (the mode switch's fail-fast branch) is the shape `ASSIST_MODE=anthropic` without `ANTHROPIC_API_KEY` must follow: fail at `config.Load()`, never at first request.
- **Op validation** — consumes `internal/document`'s primitives: `parse.go:132` (`ParseAndValidate` — the parse→validate entry shape to mirror for `ParseAndValidateOpBatch`), `validate.go:28` (`Validate` — the caller `internal/assist` calls into for the reused stroke checks).
- **No rate-limit infra exists yet** — `go.mod` has no `golang.org/x/time/rate` and no existing limiter package; the per-user token bucket (ASSIST.md §3.4) is genuinely new code in `internal/assist` (a small in-process `map[userID]*bucket` behind a mutex is sufficient for Phase A — no new dependency required, a bucket is ~20 lines).
- **Gotcha: `Retry-After` ordering.** Set the `Retry-After` header **before** calling `web.Error` — `web.Error` calls `JSON`, which calls `w.WriteHeader(status)` (`web.go:23-25`); headers set after `WriteHeader` are silently dropped by `net/http`.
- **Assist is stateless** — no DB reads/writes, no new migration, no new sqlc queries. Every request is self-contained (`prompt` + `docSummary` in, `ops` out).
- **`.env.example:14-21`** (the `RENDER_MODE` block: comment explaining the modes, the var, the commented-out conditional vars) — mirror this exact shape for the new `ASSIST_MODE`/`ANTHROPIC_API_KEY`/`ASSIST_MODEL` block.

### 2.4 Web (`apps/web`)

- **`drawings.ts:98-121`** (the `drawings` object — typed methods over `request<T>()`) — the fetch-client shape to copy into a new `apps/web/src/core/api/assist.ts` (`generateOps(prompt, docSummary, targetLayerId?)`).
- **`http.ts:13-56`** (`ApiErrorCode` union + `ApiError` class + `isAuthError`) — `validation_failed`/`rate_limited`/`unauthorized` are already mapped to 400/429/401 by the generic `request()` machinery; no client-side changes needed there, the assist mutation just handles the thrown `ApiError` like any other.
- **`queries.ts:32-52`** (`useSaveDrawing` + `useLoadLatestDrawing` — the TanStack mutation wrapper shape) — add `useAssist()` the same way: a thin `useMutation` wrapping `assist.generateOps`.
- **`DrawView.vue:66`** (`let editor: Editor | null = null`) — the module-scope editor ref the new prompt-panel handlers close over.
- **`DrawView.vue:303-346`** (`onMounted`/`onBeforeUnmount`) — no structural change needed here; the ghost lives inside `packages/editor` (§2.2), so the view doesn't need its own Konva teardown for it.
- **`DrawView.vue:564-595`** (`save`/`load` — the `mutate(...)` + `onSuccess`/`onError` shape) — mirror for the assist submit handler (`useAssist().mutate({prompt, docSummary: buildSummary(), targetLayerId}, {onSuccess: (r) => editor?.previewOps(r.ops), onError: (e) => reportError(e, 'generate')})`).
- **`DrawView.vue:547-562`** (`reportError`) — reuse verbatim; a 401 from the assist endpoint opens the auth drawer exactly like a failed save.
- **`DrawView.vue:604-635`** (the `#top-left` actions island) — natural home for an assist **toggle** (open/close the prompt panel), following the existing `IconButton` + `OriSurface` island pattern already there (help/layers).
- **`EditorShell.vue:47-64`** (the region-slot block; `#top-center` specifically at `:50-52`) — `#top-center` is a free, unused region slot today (nothing currently fills it) — mount the prompt input panel there; it's already `pointer-events: none` on the strip with content expected to opt back in (see `.draw__toolbar-item` in `DrawView.vue` for the opt-in pattern).
- **`IconButton.vue`** + **`ToolIcon.vue:6-56`** (the `IconName` union at `:6-28` + the matching `ICONS` glyph record at `:33-56`) — add an `'assist'` (or similar) entry to both in the same change; `IconButton` itself needs no change, it already takes any `IconName`.
- **`JpFloat` is deleted** (`docs/DESIGN-SYSTEM.md:101,125`) — the prompt panel and Accept/Reject controls use `OriSurface` + `IconButton`/`OriButton` directly, never a resurrected `JpFloat`.

### 2.5 Docs (update in lockstep with the code, same change)

- `docs/ASSIST.md` — fold in the three resolutions (§1) and rename `layer_id`/`doc_summary`/`target_layer_id` to camelCase; rename the editor API section to the real method names (`previewOps`/`acceptOps`/`rejectOps`).
- `docs/API.md` — add a `POST /api/assist/ops` row (new subsection under §8 or a new §8a, owner's call at merge time) documenting the request/response shape, the `400 validation_failed` retry-exhaustion path, and the `429 rate_limited` per-user bucket.
- `docs/DOCUMENT-FORMAT.md` — a one-line cross-reference noting the Op contract is a derived, additive schema over `Stroke`/`Layer`, owned by ASSIST.md, not a document-format change itself.
- `docs/DECISIONS.md` — a new dated entry recording the three resolutions (422→400/429 reuse, `add_layer.id` + single-namespace batch refs, minimal `DocSummary`) and why they correct the 2026-07-07 accepted design.
- `docs/ROADMAP.md` — tick AI Assist Phase A under Phase 4 (or promote it to its own tracked line) once shipped; note fake-first / real-impl-scaffolded status.
- `docs/NOTES.md` — gotchas worth banking: the `Retry-After`-before-`web.Error` ordering, the running-top-index fix for multi-`add_layer` batches, the ghost-remount-on-rerender discipline.

## 3. Gotchas

- **Composite invert must run children in reverse.** `add_layer` then `add_stroke` on that layer, accepted as one batch: undo must remove the stroke before the layer, or `invert` runs against a document that no longer has the layer the stroke command expects.
- **`addLayerCommand`'s index clamps at *apply* time** (`history.ts:65`), not at construction — a batch with several `add_layer` ops needs a running top-of-stack index threaded through construction, or every new layer in the batch clamps to the same insertion point.
- **The ghost overlay must follow the cursor-overlay remount discipline** (`editor.ts:531-556`, `:549`) or it silently vanishes the next time `rerender()` runs (any commit, undo, redo, or `loadDocument`) — `rerender` destroys and rebuilds the whole Konva stage.
- **`freehand` passes the existing stroke validator** — the op-level exclusion (§1 smaller corrections) must be an explicit allowlist check in the Op validator on both sides, not an assumption that reusing `validateStroke`/`checkStroke` already forbids it.
- **Go zero-fills absent JSON fields** — an absent `add_layer.name` decodes to `""`, not an error, unless the `requiredKeys`-style presence guard (`parse.go:154-201`) is mirrored for ops.
- **The assist endpoint only has the summary, not the whole document** — it can enforce per-stroke and per-batch caps, but never whole-doc caps (`maxLayers`/`maxStrokes`/`maxTotalPoints`); those still fire only at the eventual save write-edge.
- **`packages/*` dist must be rebuilt before `apps/web` sees changes** — no HMR across the package boundary (`CLAUDE.md`, `docs/NOTES.md`); a stale `packages/editor/dist` means `previewOps`/`acceptOps`/`rejectOps` silently don't exist to the app.
- **Parity discipline is non-negotiable and single-change**: the Op schema lands in `docs/ASSIST.md` + `packages/document` + `server/internal/document` + `ops.test.ts` + `ops_test.go` in **one** commit/PR, with **identical test case names** on both sides — the same discipline the Stroke contract already follows.

## 4. Orchestration & verification

- **Contract first, keystone discipline.** Land §2.1 (TS + Go validators + mirrored test tables) on its own before Editor/Server/Web start — both downstream branches import types/validators from it.
- **Tiered delegation.** Route the mechanical mirroring work (TS↔Go table copies, DTO camelCasing, config-switch boilerplate) to `sonnet`-tier subagents; reserve `opus` for the ghost-overlay/composite-command design in `packages/editor` and the retry/validate loop in `internal/assist`, where a wrong call is expensive to unwind.
- **Parallel branches after the keystone merges**: Editor (`packages/editor`) and Server (`server/internal/assist`) share no files and proceed in parallel; Web depends on both (needs the built editor API + the live endpoint) and follows. All merge `--no-ff` per `CONTRIBUTING.md`.
- **Gates before merge**: `npm run build` / `npm run types` / `npm run test` (root, fans out to all TS workspaces) + `go build ./...` / `go vet ./...` / `go test ./...` (in `server/`). Every gate must pass headless — no DB, no `ANTHROPIC_API_KEY` — since `ASSIST_MODE=fake` and the document validators are pure.
- **Final adversarial review**, same lenses as any cross-cutting change: `jp-contract-parity` (the Op schema 1:1 across TS/Go/docs), `jp-go` (the new `internal/assist` package + rate-limit code), `jp-security` (auth on the route, the rate limiter, no document data leaking into the LLM prompt beyond the summary), `jp-frontend` (dependency direction — `packages/editor` still imports nothing from Vue/the app), `jp-scope-guard` (Phase A stays `add_layer`/`add_stroke` only — no `update_stroke`/`delete_stroke` creep from ASSIST.md's v2 placeholder).
- **Live verification** needs both dev servers running with `ASSIST_MODE=fake` (no API key required): draw something on `/draw` → open the prompt panel → submit a prompt → confirm the ghost preview renders (reduced opacity, accent outline, not in the document) → Accept → confirm the batch lands as strokes/layers AND exactly **one** `Ctrl+Z` removes the whole thing.

---

*Files touched: `packages/document/src/{types.ts, validate.ts, constants.ts, index.ts}` + `test/ops.test.ts` (new); `server/internal/document/{document.go or a new op.go, parse.go, validate.go}` + `ops_test.go` (new); `packages/editor/src/{history.ts, editor.ts, index.ts}`; `server/internal/assist/*` (new package: `assist.go` interface, `fake.go`, `anthropic.go` scaffold, `handler.go`, `ratelimit.go`); `server/internal/platform/config/config.go`; `server/cmd/server/main.go`; `server/.env.example`; `apps/web/src/core/api/{assist.ts (new), queries.ts, http.ts}`; `apps/web/src/views/DrawView.vue`; `apps/web/src/components/shell/EditorShell.vue`; `apps/web/src/components/ui/IconButton.vue`; `apps/web/src/components/icons/ToolIcon.vue`. Docs updated in lockstep: `docs/ASSIST.md`, `docs/API.md` §8/new, `docs/DOCUMENT-FORMAT.md`, `docs/DECISIONS.md`, `docs/ROADMAP.md`, `docs/NOTES.md`.*
