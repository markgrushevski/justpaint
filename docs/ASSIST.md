# AI Assist — text drawing commands

> **The design for AI inside the product, phase one.** A natural-language prompt ("draw a house with a red roof") goes to an LLM, the LLM emits a batch of **validated document operations**, and the batch is applied through the existing command seam (`packages/editor/src/history.ts`). This implements item (a) of `IDEAS.md` "AI inside the product" (text drawing commands); the canvas co-author and AI inpainting are later phases over the same seam. This document owns the Op contract, the `internal/assist` server module, the doc-summary shape, and the ghost-preview UX.
>
> **Ownership note.** The document schema and its invariants stay owned by `DOCUMENT-FORMAT.md` and the two validators; the HTTP envelope/status conventions by `API.md`; the command/undo model by `packages/editor`. This doc only defines what the LLM is allowed to say and how it gets applied.
>
> **Status:** v1 design, accepted 2026-07-07 (`DECISIONS.md`). Phase A build in progress on `feat/assist-phase-a`.
>
> **Amended 2026-07-13 (Phase A build):** the three resolutions in `DESIGN-ASSIST-PHASE-A.md` §1 are folded in — `add_layer` gains an `id`, all DTOs are **camelCase**, the `DocSummary` is **minimal** (§4), and retry-exhaustion returns **`400 validation_failed`** not `422` (§3.3). §2 and §4 below reflect the **shipped** contract (`packages/document` + `server/internal/document`); §3 (endpoint) and §5 (client) are reconciled as those phases land.

## 1. Overview

```
prompt ──> POST /api/assist/ops ──> LLM (structured output) ──> ops[]
                │                                                 │
                └── validated server-side (Go document validator) ┘
                                                                  │
client: ops ──> ghost preview ──> Accept ──> one composite Command ──> Editor.acceptOps()
```

- **The LLM speaks document language, never Konva.** It emits operations over the vector document (`Document/Layer/Stroke`), which the existing validators can check and the existing command history can apply/invert. Rendering stays Konva's job, exactly as for human input.
- **The command seam does the heavy lifting.** Because `/draw` is already command-based (`Command {apply, invert}` in `packages/editor/src/history.ts`), an accepted AI batch is just one more command: undo/redo comes free, nothing new touches persistence.
- **The API key stays server-side.** The call is proxied through a new Go module (`server/internal/assist`) — that is the main reason the browser doesn't call Anthropic directly. `ANTHROPIC_API_KEY` never reaches the client.

## 2. The Op contract (canonical)

Ops are a discriminated union, deliberately small in v1:

```ts
type Op =
  | { kind: "add_layer"; id: string; name: string }
  | { kind: "add_stroke"; layerId: string; stroke: Stroke };

// v2 (design placeholder — NOT in the v1 schema):
//  | { kind: "update_stroke"; id: string; patch: StrokePatch }
//  | { kind: "delete_stroke"; id: string }
```

**Pinned v1 rules:**

| Rule | Why |
|---|---|
| `stroke` is restricted to `line \| rect \| ellipse \| polygon` | **Freehand is excluded in v1**: LLM point-path generation is low quality (jittery, self-intersecting paths); `polygon` already covers arbitrary shapes. Freehand generation is Phase C. |
| `add_layer` carries an LLM-assigned **`id`**; `add_stroke.layerId` resolves against (existing summary layer ids) ∪ (`add_layer` ids **earlier in the same batch**), in array order | Lets one prompt "add a layer, then draw on it" without a round-trip. The `id` lives in the single id-namespace (deduped like any layer/stroke id); a **dangling or forward** reference is a validation failure. |
| Every produced stroke passes the existing per-stroke validators; the batch is capped at `maxOpsPerBatch` (**64**) | The Op schema adds no new stroke invariants — it composes the existing `Stroke`/`Layer` contract. The endpoint sees only the doc summary, so **whole-document** caps (maxLayers/maxStrokes/maxTotalPoints) fire at the drawings save write-edge, not here — the op validator enforces per-stroke + per-batch caps. |
| The Op schema lives in **both** validators, 1:1 | Same dual-contract discipline as the Stroke contract: `packages/document/src/validate.ts` (TS, client) and `server/internal/document` (Go, server) must mirror every invariant, with mirrored test tables. A schema change lands in this doc AND both validators AND both test tables together. |

`update_stroke` / `delete_stroke` are **v2**: they require the LLM to reference existing stroke ids from the doc summary (§4), which only pays off with iterative chat (Phase B). The union is designed so adding them is additive — new `kind` values, no change to v1 ops.

## 3. Server — `internal/assist` (a judge-style seam)

A new Go module `server/internal/assist/` mirroring the `internal/judge` seam pattern:

- an **`Assist` interface** — the one thing handlers and tests depend on;
- a **fake implementation** (canned ops, deterministic) — the default in dev/CI/tests, exactly like `FakeJudge`;
- a **real implementation** on the official Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`), selected by config at composition time.

### 3.1 Endpoint

`POST /api/assist/ops` — **auth required** (session cookie, like every write route). Full HTTP contract (status map, byte cap): `docs/API.md` §10.

```jsonc
// request
{
  "prompt": "draw a house with a red roof",
  "docSummary": { /* §4 */ },
  "targetLayerId": "l1"        // optional: bias generation onto this layer
}

// response
{
  "ops": [ /* Op[] — §2 */ ],
  "note": "Drew the house as a rect body, polygon roof, and two rect windows." // optional, surfaced in the UI
}
```

All DTOs are **camelCase**, matching the rest of the live API.

### 3.2 The LLM call (pinned choices)

- **Mode switch, mirroring `RENDER_MODE`.** `ASSIST_MODE` selects the impl at composition time: **`fake`** (default — deterministic canned ops, zero API dependency; dev/CI default) or **`anthropic`** (the real LLM impl). `ANTHROPIC_API_KEY` is the server-side key, required (fail-fast at `config.Load()`) when `ASSIST_MODE=anthropic`; it never reaches the client.
- **`client.Messages.New`, non-streaming.** Op batches are small; streaming buys nothing in v1 (a non-goal, §8).
- **Structured outputs** via `output_config.format` json_schema — in Go: `OutputConfig: anthropic.OutputConfigParam{Format: anthropic.JSONOutputFormatParam{Schema: ...}}`. The schema is the Op-batch schema with `additionalProperties: false` everywhere and **no recursion** — this guarantees parseable JSON matching the Op schema, so the retry path (§3.3) only ever deals with *semantic* validation failures, never JSON syntax.
- **`MaxTokens: 8000`.**
- **Model: `claude-opus-4-8`** by default — best shape-composition quality — configurable via **`ASSIST_MODEL`**; `claude-haiku-4-5` ($1/$5 per MTok) is the documented budget option. Optionally `output_config.effort: "low"` for simple command generation.
- **Phase A ships `fake`-first.** `ASSIST_MODE=anthropic` is scaffolded (config plumbing + a stub that fails fast with "not implemented") but not wired to a live SDK call — no API key exists yet. `docs/DESIGN-ASSIST-PHASE-A.md` §1 owns this scoping.

### 3.3 Validate → retry → `400`

The server **validates every op** against the Go document validator before returning anything to the client:

1. Validate the returned batch (schema parity + document invariants + intra-batch layer refs).
2. On failure: **one retry**, with the validator errors appended to the prompt ("your previous output failed validation: …").
3. Still invalid → **`400 validation_failed`** with the reason in the error envelope — **not `422`**, which `docs/API.md` reserves unused in v1 (one consistent client path for doc-invalid). The client never receives unvalidated ops; the handler re-validates the returned batch defense-in-depth before responding even on the impl's reported success.

### 3.4 Rate limiting

A **per-user token bucket** on `/api/assist/ops` ships *with* this feature (not deferred like the general 429 work in `DECISIONS.md` 2026-06-20) — the app will be a public demo and each request costs real API money. Simple in-process bucket, keyed by user id; the general per-IP/per-login limiter can absorb it later. Exceeding it returns `429 rate_limited` with a `Retry-After` header (seconds).

## 4. The doc summary (token thrift)

The full document jsonb is **never** sent to the LLM (a max doc is ~2.5–3 MB — pure token waste, and freehand point arrays are noise to a shape-composing model). The client sends a compact summary:

```ts
interface DocSummary {
  canvas: { width: number; height: number };
  layers: Array<{ id: string; name: string; strokeCount: number }>;
}
// Phase A ships this MINIMAL shape (the fake Assist ignores the summary). Per-stroke
// `bbox` / `recent_strokes` / a `style` projection are deferred until a real
// prompt-composition need justifies the token cost (DESIGN-ASSIST-PHASE-A.md §1, res. 3).
```

This gives the LLM the canvas size and the layer inventory without ever sending point paths. (Phase A stops here; per-stroke bboxes / recent strokes / styling — the richer signals for *placing* shapes — are a deferred enrichment the deterministic fake Assist doesn't need. Stroke ids, once summarized, are what v2 edit ops will reference.)

## 5. Client (`apps/web`)

- **Transport:** `useAssist()` (`core/api/queries.ts`) — a TanStack Query mutation wrapping the typed `assist.ops` client (`core/api/assist.ts`), same pattern as save/load.
- **UI:** a prompt input panel in `DrawView.vue`, mounted in the shared `EditorShell`'s `#top-center` region (a free region slot the shell already reserves) and toggled open/closed from an `assist` icon in the top-left actions island, alongside the layers/help toggles.
- **Ghost preview:** returned ops are handed to `Editor.previewOps(ops)`, which renders them on a ghost overlay — its own top, non-listening Konva layer at reduced opacity with a dashed accent frame, clipped to the doc rect — drawn on the stage but **not in the document and not in history**. The user then:
  - **Accept** — `Editor.acceptOps()` maps the whole batch into a **single composite `Command {apply, invert}`** and commits it through the editor's normal commit path. The entire AI action is one history entry: **one Ctrl+Z undoes all of it**.
  - **Reject** — `Editor.rejectOps()` discards the preview; nothing enters the document or history.
- This keeps the trust and UX boundary crisp: AI output is a *proposal* until the user accepts, and an accepted proposal is indistinguishable from any other command in the history model.

## 6. Testing

Same playbook as the judge seam:

- **Fake `Assist`** with canned op batches — dev/CI default; the whole client flow is demonstrable with zero API dependency.
- **Go table tests:** op validation (schema parity, intra-batch layer refs, document invariants), the retry path (invalid → retry with errors → valid, and invalid → invalid → 400), auth + rate-limit behavior.
- **Vitest:** ops → composite `Command` mapping (apply/invert round-trip), ghost preview accept/reject.
- **Contract-parity test** for the Op schema (TS vs Go) — mirrored test tables, exactly like the Stroke contract.

## 7. Phasing

- **Phase A (MVP):** v1 ops (`add_layer`, `add_stroke`), ghost preview + accept/reject, fake + real `Assist` impls, the prompt panel in `/draw`.
- **Phase B:** edit ops (`update_stroke`, `delete_stroke`); iterative chat that references existing stroke ids from the doc summary.
- **Phase C:** freehand generation; **AI inpainting** via the render worker (`renderToPNG`) + an image API — requires an image/raster stroke type in the document contract (a separate decision, format §9 additive-field rules); a **real judge implementation** for `/play` reusing the same internal Anthropic client plumbing.

## 8. Non-goals (v1)

- **Streaming** — batches are small; `Messages.New` non-streaming is fine.
- **Multi-turn conversation memory** — each prompt is independent; iterative chat is Phase B.
- **Freehand generation** — excluded from the v1 op schema (§2); Phase C.
- **Image generation / inpainting** — Phase C, gated on a raster stroke type in the document contract.
