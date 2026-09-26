# AI Assist — text drawing commands

> **The design for AI inside the product, phase one.** A natural-language prompt ("draw a house with a red roof") goes to an LLM, the LLM emits a batch of **validated document operations**, and the batch is applied through the existing command seam (`packages/editor/src/history.ts`). This implements item (a) of `IDEAS.md` "AI inside the product" (text drawing commands); the canvas co-author and AI inpainting are later phases over the same seam. This document owns the Op contract, the `internal/assist` server module, the doc-summary shape, and the ghost-preview UX.
>
> **Ownership note.** The document schema and its invariants stay owned by `DOCUMENT-FORMAT.md` and the two validators; the HTTP envelope/status conventions by `API.md`; the command/undo model by `packages/editor`. This doc only defines what the LLM is allowed to say and how it gets applied.
>
> `ASSIST_MODE=fake` (default) selects `FakeAssist`; `ASSIST_MODE=gemini` selects `GeminiAssist` (`server/internal/assist/gemini.go`), which reaches the same API — and the same key, quota and HTTP client — as the judge, the critic and the guesser (`judge.GeminiClient`, `JUDGE.md` §8.1).

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
- **The API key stays server-side.** The call is proxied through a new Go module (`server/internal/assist`) — that is the main reason the browser doesn't call the provider directly. `GEMINI_API_KEY` never reaches the client, and it travels in the `x-goog-api-key` **header**, never in a URL (§3.2).

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
| The Op schema is validated on the server only | `server/internal/document` (`ValidateOpBatch`) enforces every invariant; the TS `Op` type mirrors this doc. A schema change lands in this doc, the Go validator and its tests, and the TS type together. |

`update_stroke` / `delete_stroke` are **v2**: they require the LLM to reference existing stroke ids from the doc summary (§4), which only pays off with iterative chat (Phase B). The union is designed so adding them is additive — new `kind` values, no change to v1 ops.

## 3. Server — `internal/assist` (a judge-style seam)

A new Go module `server/internal/assist/` mirroring the `internal/judge` seam pattern:

- an **`Assist` interface** — the one thing handlers and tests depend on;
- a **fake implementation** (canned ops, deterministic) — the default in dev/CI/tests, exactly like `FakeJudge`;
- a **real implementation** (`GeminiAssist`), selected by config at composition time. It is **stdlib only**: it embeds `judge.GeminiClient` — one `generateContent` endpoint, the header-carried key, the per-attempt deadline and the `JUDGE.md` §7 retry policy — rather than adding a second vendor's SDK for the same job. What it takes from `internal/judge` is the **wire and nothing else**: no `Judge`, no `Critic`, no §2 contract. The external ML judge may take this seam later, which is the whole reason it is an interface.

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

**Mode switch, mirroring `RENDER_MODE`.** `ASSIST_MODE` selects the impl at composition time:

| `ASSIST_MODE` | Impl | What it is |
|---|---|---|
| `fake` (default) | `FakeAssist` | The same canned op batch whatever the user typed — a prompt box that ignores the prompt. Zero API dependency, right in dev and CI, **not** a thing to ship. |
| `gemini` | `GeminiAssist` | The real one: the prompt really becomes shapes. Requires `GEMINI_API_KEY` (fail-fast at `config.Load()`); it never reaches the client. |

There are exactly two modes, `fake` and `gemini`. An unrecognized `ASSIST_MODE` is **refused by name at boot**, not silently downgraded to `fake`: a fake that boots green while answering every prompt with the same canned house is a dead feature wearing a live one's face.

`ASSIST_MODE` is read **independently of `JUDGE_MODE`**. They share a key and a quota but not a decision: a real prompt box on `/draw` alongside a fake judge is a perfectly sensible deployment, and the two switches never consult each other.

**What `GeminiAssist` sends:**

- **One `generateContent` call per attempt, text only** — no image part, since the endpoint sees only the doc summary (§4). Non-streaming; op batches are small and streaming buys nothing (a non-goal, §8).
- **Structured output**, exactly as the judge does it: `responseMimeType: "application/json"` plus a `responseSchema`, and `temperature: 0` (pinned by the shared client, not an argument a caller may pass — a caller that wants a different answer has to ask a different question).
- **The model is the same knob every Gemini seam reads**: `GEMINI_MODEL` (default `gemini-3.6-flash`), overridden for this kind alone by `AI_MODEL_PER_KIND=assist=…`; `GEMINI_BASE_URL` for the API root. Assist gets **no model knob of its own**, because the quota assist spends is counted per provider **and model** (`GAME.md` §4.3), so the model id and the budget key have to be the same fact, and a second knob could only ever disagree with it.
- **`maxOutputTokens: 8192`**, and this one is not a preference. Left unset, a thinking model spends its whole default output budget reasoning and runs out partway through the shape list; the API then **closes the JSON so it stays parseable**, and what arrives is a syntactically perfect object whose last shape is half written (`{"type":"rect","x":0,"y":0` and then the brackets). It fails validation as a zero-area rect and reads exactly like a model that cannot draw a house. The impl checks `GeminiOutput.Finish == "MAX_TOKENS"` and says so in the retry (§3.3). The number is deliberately loose: a whole house costs ~450 tokens, and the budget also pays for the thinking.
- **Thinking is left alone** — no `thinkingConfig` on the wire. Pinning the budget to zero measurably saved a few seconds, and it was still dropped: it is the newest and least portable field we could send, in a service whose operator is invited to change models per kind, and a model that refuses it rejects the whole request.

**The model does not emit `Op`s. It emits a flat list of primitive shapes, and the Go side expands them.** That is the load-bearing choice in this file:

```jsonc
// what the model returns
{
  "layerName": "House",
  "note": "A house: a rectangular body, a triangular roof, a door and two windows.",
  "shapes": [
    { "type": "rect", "x": 300, "y": 500, "width": 480, "height": 400,
      "cx": 0, "cy": 0, "rx": 0, "ry": 0, "points": [],
      "fill": "#e8d7b5", "stroke": "#4a3b2a", "strokeWidth": 6 },
    { "type": "polygon", "points": [270, 500, 540, 300, 810, 500], /* … */ }
  ]
}
```

- **The model never invents an id, never names a layer reference and never picks a composite.** `expand()` mints the layer (`ai-<48 bits of hex>`, re-rolled up to 5 times against the summary's ids) and one stroke id per shape, emits the `add_layer` **first** so the `add_stroke.layerId` resolves in array order (§2), and pins `composite: "source-over"` — an AI proposal that could *erase* what is under it is a different and much less welcome feature. Those are the invariants that are ours to get right, so the whole "duplicate id" / "unknown layer id" class of failure is unreachable rather than merely unlikely.
- **It is always a NEW layer, even when `targetLayerId` is set.** §3.1 calls that field a *bias*, and the client sends whatever layer happens to be active, so honouring it literally would mean the AI always drew into the user's current work. A batch that brings its own layer is also what makes Accept clean: one composite command, one Ctrl+Z, nothing of the user's touched. The target still reaches the model as *context* — which is what "bias" means.
- **Points travel as one FLAT integer array**, `[x1,y1,x2,y2,…]`, reshaped into the document's pairs server-side. The document's `Point` is a nested array and the schema could probably express that; a flat list has one failure mode we can name exactly (*"points has 7 values, which is not a whole number of x,y pairs"*) instead of an arity the schema cannot pin anyway — `GeminiSchema` carries no `minItems`/`maxItems`, deliberately (`JUDGE.md` §8.1).
- **The four shape types are `rect` / `ellipse` / `polygon` / `line`**, as a schema `enum` — the Op contract's stroke set minus freehand (§2). The union is **flattened into one object** with every geometry field present, because the schema has no `oneOf`: a `rect` reads `x/y/width/height` and ignores `cx/cy/rx/ry`. The cost is a model free to fill a field its own type does not use, which `toStroke` drops; `""` reads as an absent colour and `0` as an absent width, because that is how structured output spells "absent" and the contract spells it with a missing field. A `line` is the one shape whose stroke colour and width are *required* by the document contract with no absent spelling, so a missing one falls back to `#1a1a1a` at width `4` rather than failing the batch.
- **Coordinates are `INTEGER`, not `NUMBER`** — see the trap in `docs/NOTES.md`; a canvas coordinate is a pixel, so it costs nothing, and it removes a decoding loop from the grammar rather than making it unlikely.
- **Every shape field is `required`**, including the ones a given type ignores — also a measured fix, also in `NOTES.md`. An "optional" field over structured output is not a field the model weighs; it is one it is free to skip.
- **At most 24 shapes** (`maxShapesPerBatch`), stated in both the instruction and the schema description, with a test asserting the two agree. This is a **quality request, not the cap**: the contract's `maxOpsPerBatch` is 64, so 63 shapes would still validate. A recognisable drawing comes from a dozen deliberate shapes, not sixty small ones; a model that overruns the real cap is caught by `ValidateOpBatch` and told so on the retry.
- **The note is clamped to 200 runes and the layer name to 64**, on a rune boundary, never rejected. They are model-authored prose beside a preview and a label on a layer list: they decide nothing, and throwing away a whole drawing over a long label would be absurd.

**Timeout: `ASSIST_TIMEOUT`, default `60s`, and deliberately NOT `JUDGE_TIMEOUT`.** It bounds one attempt, and the shared client may make three (`JUDGE.md` §7) before the two batch attempts of §3.3 even begin. Folding it into the judge's knob was the first attempt and it was wrong in both directions: a verdict is four scalars and lands in a second or two, while composing a picture is a list of shapes from a model that reasons first — measured **4–9s healthy** and far longer when Google is busy (2026-09-20) — so the judge's `10s` default leaves no headroom at all, and raising the *shared* knob to fit would hand the duel a retry envelope that no longer fits inside `game.JudgePassBudget`. The cost of a loose bound is one wedged call holding a request goroutine for a minute; the cost of a tight one is a feature that fails whenever the provider is slow, which is the failure nobody can diagnose from outside.

**Prompt-injection surface — and this is the first seam where the untrusted thing is TEXT.** Everywhere else the model is handed a prompt of ours plus pixels a player drew (`JUDGE.md` §8.1, §8.3); here the user's own sentence goes into the user turn, which is the channel a model takes instructions on. The mitigations are the ordinary ones: the rules live in the **system** instruction so the untrusted text necessarily arrives after them, the request is placed **last** in the user turn, behind a label and `%q`-delimited (as are the user-authored **layer names** in the summary), and the instruction says plainly that everything inside it is a description of a picture and never an instruction — including if it claims new rules, claims authority, claims to be from the developers, or asks what the instructions say.

Be honest about what that buys: it **narrows** the surface, it is not a boundary, and no instruction ever was one against sufficiently determined text. What bounds this is the blast radius, and here it is unusually small — the model holds no credentials, calls no tools, reads no database, and is shown nothing but the user's own prompt and the size and layer names of their own canvas. Every op it returns is validated against the document contract **twice** (in the impl, as the retry's condition, and again in the handler), and the client renders the result as a **ghost the user has to accept** (§5). The worst a successful injection buys is a drawing the person who typed the prompt did not want, on their own canvas, which they decline with one click.

**Verification, stated exactly.** The whole path — the two-deep `ARRAY` schema, the flat points, the expansion, a live batch passing `ValidateOpBatch` — was exercised against the real API on 2026-09-20 (`gemini_live_test.go`, opt-in behind `GEMINI_LIVE=1` so a stray key can never quietly spend the daily quota). **It was not verified on `gemini-3.6-flash`, which is the configured default.** That model's free-tier pool went on the diagnostics above and it also answered `503 "high demand"`; the runs that succeeded used other models (`gemini-3.1-flash-lite`, `gemini-3.7-flash`) through the **identical** code path. Treat the default as unproven end to end until someone spends a fresh day's quota on it.

### 3.3 Validate → retry → `400`

The server **validates every op** against the Go document validator before returning anything to the client:

1. Validate the returned batch (schema parity + document invariants + intra-batch layer refs), inside the impl.
2. On failure: **one retry** — `geminiAssistAttempts` is 2, one try plus one — with the validator's own complaint appended to the prompt (*"Your previous answer could not be drawn: …. That is a rule of the canvas, not a matter of taste. Return the same picture with that fixed."*). The retry is **not a second roll of the dice**: temperature is 0, so an unchanged prompt buys the same answer, and the appended complaint is the only lever there is. It is two and not more because the ledger records **one** assist call per request whatever happens in here, exactly as it records one duel per judging pass and not one per `JUDGE.md` §7 transport retry — a generous retry budget would spend quota the ceiling cannot see. When the answer came back `MAX_TOKENS`, the complaint says so first, because a truncated answer fails validation on its half-written last shape and otherwise reads as a baffling complaint about a zero-width rect.
3. Still invalid → **`400 validation_failed`** with the reason in the error envelope — **not `422`**, which `docs/API.md` reserves unused in v1 (one consistent client path for doc-invalid). The cause travels with `ErrInvalidBatch` into the log, where a model that keeps breaking the same invariant is the signal that the *instruction* is wrong, not the user.
4. The client never receives unvalidated ops: the handler re-validates the returned batch defense-in-depth before responding even on the impl's reported success. The two passes are not redundant — the handler's is depth against any impl, the impl's is the retry's *condition*.

The transport failures underneath are **final on sight** and never re-asked: the shared client has already retried what is worth retrying (`JUDGE.md` §7 — connection errors, timeouts, 5xx, never a 4xx), so a quota refusal, a safety block or an unreadable `200` returns straight out. One of them is mapped rather than logged: **`judge.ErrQuotaExhausted` answers `429`**, the same refusal our own global ceiling writes, matching practice and guess (`JUDGE.md` §8.1, `API.md` §10). The provider ran out before we did — the same news from the other end — and a `500` would invite a retry that cannot succeed until the provider's window rolls.

### 3.4 Rate limiting

**Two limits, answering two different questions.** A **per-user token bucket** on `/api/assist/ops` ships *with* this feature (not deferred like the general per-IP 429 work, `DECISIONS.md` 2026-09-18) — the app will be a public demo and each request costs real API money. Simple in-process bucket, keyed by user id; the general per-IP/per-login limiter can absorb it later. Exceeding it returns `429 rate_limited` with a `Retry-After` header (seconds).

The bucket bounds the **rate** — how fast one user may ask — and it lives *in this process*, so the host resets it on every deploy and every wake from idle. That means it was never a ceiling at all: it could be emptied, and then handed back in full, as often as the instance restarted. Since **2026-09-20** assist therefore also sits under the **daily AI-call budget** (`docs/GAME.md` §4.3, `server/internal/aibudget`), which bounds the daily **quota**, lives in Postgres, and so actually holds: kind `assist`, default **40 calls per player per rolling 24h**, plus the global ceiling counted per provider **and model**. It is enforced only when the impl really calls a provider — and the **impl is asked**, never inferred from the mode (`assist.CallsProvider`). `GeminiAssist` answers *true*, which is what finally gives assist a ceiling that survives a restart; `FakeAssist` is offline by construction, answers *false*, and is never billed. Under `ASSIST_MODE=gemini` the provider key is `google:<model>` — the same pool the judge, critic or guesser draw on **only if they run on the same model** (§3.2), which is the point of keying it that way. A mode that named a provider whose impl called nobody would be left unbudgeted and would say so at boot (`Warn`); no mode does that today, and the guard stays only because the mode used to be read as the answer — the Phase A scaffold made no call at all and was billing a ledger row for it anyway, then answering `500` (`docs/DECISIONS.md` 2026-09-20 "One AI-call ledger").

Order in the handler: the bucket first, before the body is even decoded; then the body/prompt guards; then the daily ceiling, last of the guards and immediately before the one line that costs money — the point of a quota is to refuse *before* the call, never after paying for it. The spend is recorded **before** the impl is invoked, never after: a call that fails still spent the provider's quota. That record is itself a gate — a conditional insert that writes the call's rows only while the caller is under their cap — so the same `429` can come from the write as well as from the check above it, and the two are deliberately indistinguishable on the wire. The budget's two refusals are also `429 rate_limited` but carry **no** `Retry-After` (the window rolls continuously, so there is no fixed reset to name); exact messages and status map: `docs/API.md` §10.

## 4. The doc summary (token thrift)

The full document jsonb is **never** sent to the LLM (a max doc is ~2.5–3 MB — pure token waste, and freehand point arrays are noise to a shape-composing model). The client sends a compact summary:

```ts
interface DocSummary {
  canvas: { width: number; height: number };
  layers: Array<{ id: string; name: string; strokeCount: number }>;
}
// This MINIMAL shape ships (the fake Assist ignores the summary). Per-stroke
// `bbox` / `recent_strokes` / a `style` projection are deferred until a real
// prompt-composition need justifies the token cost.
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
- **Stand-in server tests for `GeminiAssist`** (`gemini_test.go`) — they assert what we *send* (the system turn, the untrusted request last and delimited, the schema) and how we read what comes back: the happy path, the retry carrying the validator's complaint, retry exhaustion → `ErrInvalidBatch`, a `MAX_TOKENS` answer saying so, `ErrQuotaExhausted` passed through, an empty prompt refused **without** spending a call, the line defaults, the note/layer-name clamps, and ids that dodge the summary's. No quota spent, so this is what runs in CI.
- **One live test, opt-in** (`gemini_live_test.go`, `GEMINI_LIVE=1`) — the only thing that can prove Google accepts a two-deep `ARRAY` schema and that a model fills it with a drawing rather than a shrug. Read §3.2's verification note for which model that has and has not been run on.
- **Vitest:** ops → composite `Command` mapping (apply/invert round-trip), ghost preview accept/reject.
- **Contract-parity test** for the Op schema (TS vs Go) — mirrored test tables, exactly like the Stroke contract.

## 7. Phasing

- **Phase A (MVP, 2026-07-13):** v1 ops (`add_layer`, `add_stroke`), ghost preview + accept/reject, `FakeAssist`, the prompt panel in `/draw`.
- **The real impl (2026-09-20):** `GeminiAssist` behind `ASSIST_MODE=gemini` — same ops, same client, same UX, a prompt that is finally read (§3.2). The external ML judge may still take this seam later, which is what the interface is for.
- **Phase B:** edit ops (`update_stroke`, `delete_stroke`); iterative chat that references existing stroke ids from the doc summary.
- **Phase C:** freehand generation; **AI inpainting** via the render worker (`renderToPNG`) + an image API — requires an image/raster stroke type in the document contract (a separate decision, format §9 additive-field rules).

## 8. Non-goals (v1)

- **Streaming** — batches are small; one non-streaming `generateContent` is fine.
- **Multi-turn conversation memory** — each prompt is independent; iterative chat is Phase B.
- **Freehand generation** — excluded from the v1 op schema (§2); Phase C.
- **Image generation / inpainting** — Phase C, gated on a raster stroke type in the document contract.
