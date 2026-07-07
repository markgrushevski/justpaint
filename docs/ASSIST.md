# AI Assist — text drawing commands

> **The design for AI inside the product, phase one.** A natural-language prompt ("draw a house with a red roof") goes to an LLM, the LLM emits a batch of **validated document operations**, and the batch is applied through the existing command seam (`packages/editor/src/history.ts`). This implements item (a) of `IDEAS.md` "AI inside the product" (text drawing commands); the canvas co-author and AI inpainting are later phases over the same seam. This document owns the Op contract, the `internal/assist` server module, the doc-summary shape, and the ghost-preview UX.
>
> **Ownership note.** The document schema and its invariants stay owned by `DOCUMENT-FORMAT.md` and the two validators; the HTTP envelope/status conventions by `API.md`; the command/undo model by `packages/editor`. This doc only defines what the LLM is allowed to say and how it gets applied.
>
> **Status:** v1 design, accepted 2026-07-07 (`DECISIONS.md`). Not yet implemented — Phase A below is the MVP slice.

## 1. Overview

```
prompt ──> POST /api/assist/ops ──> LLM (structured output) ──> ops[]
                │                                                 │
                └── validated server-side (Go document validator) ┘
                                                                  │
client: ops ──> ghost preview ──> Accept ──> one composite Command ──> Editor.commit()
```

- **The LLM speaks document language, never Konva.** It emits operations over the vector document (`Document/Layer/Stroke`), which the existing validators can check and the existing command history can apply/invert. Rendering stays Konva's job, exactly as for human input.
- **The command seam does the heavy lifting.** Because `/draw` is already command-based (`Command {apply, invert}` in `packages/editor/src/history.ts`), an accepted AI batch is just one more command: undo/redo comes free, nothing new touches persistence.
- **The API key stays server-side.** The call is proxied through a new Go module (`server/internal/assist`) — that is the main reason the browser doesn't call Anthropic directly. `ANTHROPIC_API_KEY` never reaches the client.

## 2. The Op contract (canonical)

Ops are a discriminated union, deliberately small in v1:

```ts
type Op =
  | { kind: "add_layer"; name: string }
  | { kind: "add_stroke"; layer_id: string; stroke: Stroke };

// v2 (design placeholder — NOT in the v1 schema):
//  | { kind: "update_stroke"; id: string; patch: StrokePatch }
//  | { kind: "delete_stroke"; id: string }
```

**Pinned v1 rules:**

| Rule | Why |
|---|---|
| `stroke` is restricted to `line \| rect \| ellipse \| polygon` | **Freehand is excluded in v1**: LLM point-path generation is low quality (jittery, self-intersecting paths); `polygon` already covers arbitrary shapes. Freehand generation is Phase C. |
| Ops may reference layer ids created earlier in the **same batch** | Lets one prompt produce "add a layer, then draw on it" without a round-trip. Resolution is positional within the batch; a reference to a nonexistent id is a validation failure. |
| Every produced stroke/layer must pass the document validators | The Op schema adds no new invariants — it composes the existing `Stroke`/`Layer` contract. DoS caps (`API.md`) apply to the resulting document as usual. |
| The Op schema lives in **both** validators, 1:1 | Same dual-contract discipline as the Stroke contract: `packages/document/src/validate.ts` (TS, client) and `server/internal/document` (Go, server) must mirror every invariant, with mirrored test tables. A schema change lands in this doc AND both validators AND both test tables together. |

`update_stroke` / `delete_stroke` are **v2**: they require the LLM to reference existing stroke ids from the doc summary (§4), which only pays off with iterative chat (Phase B). The union is designed so adding them is additive — new `kind` values, no change to v1 ops.

## 3. Server — `internal/assist` (a judge-style seam)

A new Go module `server/internal/assist/` mirroring the `internal/judge` seam pattern:

- an **`Assist` interface** — the one thing handlers and tests depend on;
- a **fake implementation** (canned ops, deterministic) — the default in dev/CI/tests, exactly like `FakeJudge`;
- a **real implementation** on the official Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`), selected by config at composition time.

### 3.1 Endpoint

`POST /api/assist/ops` — **auth required** (session cookie, like every write route).

```jsonc
// request
{
  "prompt": "draw a house with a red roof",
  "doc_summary": { /* §4 */ },
  "target_layer_id": "l1"        // optional: bias generation onto this layer
}

// response
{
  "ops": [ /* Op[] — §2 */ ],
  "note": "Drew the house as a rect body, polygon roof, and two rect windows." // optional, surfaced in the UI
}
```

### 3.2 The LLM call (pinned choices)

- **`client.Messages.New`, non-streaming.** Op batches are small; streaming buys nothing in v1 (a non-goal, §8).
- **Structured outputs** via `output_config.format` json_schema — in Go: `OutputConfig: anthropic.OutputConfigParam{Format: anthropic.JSONOutputFormatParam{Schema: ...}}`. The schema is the Op-batch schema with `additionalProperties: false` everywhere and **no recursion** — this guarantees parseable JSON matching the Op schema, so the retry path (§3.3) only ever deals with *semantic* validation failures, never JSON syntax.
- **`MaxTokens: 8000`.**
- **Model: `anthropic.ModelClaudeOpus4_8`** (`claude-opus-4-8`, $5/$25 per MTok) by default — best shape-composition quality — configurable via **`ASSIST_MODEL`**; `claude-haiku-4-5` ($1/$5 per MTok) is the documented budget option. Optionally `output_config.effort: "low"` for simple command generation.
- **`ANTHROPIC_API_KEY` is server-side only** and required for the real impl (fail-fast at boot when the real mode is selected, matching the `JWT_SECRET`/`RENDER_CLI` posture).

### 3.3 Validate → retry → 422

The server **validates every op** against the Go document validator before returning anything to the client:

1. Validate the returned batch (schema parity + document invariants + intra-batch layer refs).
2. On failure: **one retry**, with the validator errors appended to the prompt ("your previous output failed validation: …").
3. Still invalid → **`422`** with the reasons in the error envelope. The client never receives unvalidated ops.

### 3.4 Rate limiting

A **per-user token bucket** on `/api/assist/ops` ships *with* this feature (not deferred like the general 429 work in `DECISIONS.md` 2026-06-20) — the app will be a public demo and each request costs real API money. Simple in-process bucket; the general per-IP/per-login limiter can absorb it later.

## 4. The doc summary (token thrift)

The full document jsonb is **never** sent to the LLM (a max doc is ~2.5–3 MB — pure token waste, and freehand point arrays are noise to a shape-composing model). The client sends a compact summary:

```ts
interface DocSummary {
  canvas: { width: number; height: number };
  layers: Array<{ id: string; name: string; stroke_count: number; bbox: BBox }>;
  /** The last N strokes (recency = relevance), freehand points elided. */
  recent_strokes: Array<{ id: string; type: StrokeType; bbox: BBox; style: StrokeStyle }>;
}
```

This is enough for the LLM to place new shapes sensibly (canvas size, what's where via bboxes, current styling) without ever parsing point paths. Stroke ids in the summary are what v2 edit ops will reference.

## 5. Client (`apps/web`)

- **Transport:** a TanStack Query mutation composable (`useAssist`) over the fetch API client, same pattern as save/load.
- **UI:** a prompt input panel in `DrawView.vue`, placed near `FloatingToolbar` (same floating shell language; exact placement is a design-pass detail).
- **Ghost preview:** returned ops render as a **ghost layer** — distinct styling (e.g. reduced opacity + accent outline), drawn on the stage but **not in the document and not in history**. The user then:
  - **Accept** — the whole batch is mapped into a **single composite `Command {apply, invert}`** and committed via `Editor.commit()`. The entire AI action is one history entry: **one Ctrl+Z undoes all of it**.
  - **Reject** — the preview is discarded; nothing enters the document or history.
- This keeps the trust and UX boundary crisp: AI output is a *proposal* until the user accepts, and an accepted proposal is indistinguishable from any other command in the history model.

## 6. Testing

Same playbook as the judge seam:

- **Fake `Assist`** with canned op batches — dev/CI default; the whole client flow is demonstrable with zero API dependency.
- **Go table tests:** op validation (schema parity, intra-batch layer refs, document invariants), the retry path (invalid → retry with errors → valid, and invalid → invalid → 422), auth + rate-limit behavior.
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
