# Vector document format

> **The keystone contract.** One schema, shared by three consumers: the **editor** (render + edit, Konva), the **Go backend** (store as `jsonb`, validate), and the **judge** (render → PNG to score). Canonical, versioned, renderer-agnostic. This doc is the source of truth — if code and this disagree, the doc wins until amended here.
>
> **Status:** v1 spec, Phase 0 (draft). Greenfield — no legacy to carry. Companion: `docs/DECISIONS.md` ("Vector document persisted as jsonb", "schema independent of Konva's internal JSON"). This file specifies the format that realizes those decisions; it does not relitigate them.

## 1. Design goals & priorities

In priority order — when they conflict, higher wins:

1. **Deterministic raster export.** The same document renders byte-identically wherever it runs (editor preview, server worker, judge frame). The judge scores rasters; non-determinism = unfair matches. This forces: explicit coordinate space, explicit background, pinned `perfect-freehand`, pinned render math (§6, §10), store-input-not-output.
2. **Trivial in both TS and Go.** Flat structs, one discriminated union, integer-friendly numbers, no clever polymorphism. The schema is the canonical TS source in `packages/document`; Go mirrors it (§7).
3. **Per-object identity for undo/redo & realtime.** Every layer and stroke has a stable `id`. Undo/redo and (later) WS conflict resolution operate on ids, not array offsets.
4. **Faithful replay.** Strokes are append-ordered; freehand keeps raw time-ordered input points. The persisted document alone replays *what* was drawn, in order. (Process-timing is an additive opt-in, §9.)
5. **Forward-compatible versioning.** Mandatory integer `version`, first field, mirrored to a DB column. Additive evolution is the default; breaking changes bump the integer.

Explicit **non-goals** for v1 (each has a forward-compatible seam, §9): text, images/bitmaps, gradients, dashed lines, clipping/masks, groups, blend modes, free transform of placed shapes, per-stroke multiplicative opacity, per-point timestamps. We don't pay for them now.

This is **not** a command/undo log (history is an editor-runtime concern, §10), **not** Konva's `Stage.toJSON()` (§6), and **not** the judge's wire format (the judge consumes pre-rendered PNGs, §10).

## 2. Coordinate system & DPR independence

**Fixed integer logical canvas.** A document defines a logical drawing surface of `width × height` logical pixels. Origin top-left, **x → right, y → down** (matches Konva, the 2D canvas, and the screen). All coordinates, radii, sizes, and stroke widths are in these **logical units**. There is exactly one coordinate space; layers do not have their own.

- **Default canvas: `1920 × 1080`.** A document may carry any positive integer size; consumers read `width`/`height` and never assume. The game (`/play`) pins its own canonical canvas in `docs/GAME.md` so both duelists share one space.
- **No device pixels, no DPR, ever, in the document.** Capture pointer input in stage-local logical space — Konva's `stage.getRelativePointerPosition()` already returns logical coords; never use `pageX - offsetLeft` (the old engine's bug, `client/src/modules/canvas/models/CanvasTool.ts:257-269`). The renderer scales logical → device at draw time via `pixelRatio`; that scale never touches stored data. This is precisely what makes a document render identically at any DPR or output size.
- **Precision.** Store as JSON numbers; round coordinates to **2 decimals**, pressure to **3** on write (sub-logical-pixel precision is meaningless and bloats jsonb). All numbers must be **finite** — the Go validator rejects `NaN`/`±Infinity`. Fractional values are fine; coordinates need not be clamped to the canvas (strokes may bleed past the edge; clipped at render).

**Why fixed logical canvas over normalized `0..1`:** integer-friendly math, direct 1:1 mapping onto Konva attrs (no multiply-by-size on every read), simpler debugging. Any consumer that wants normalized coords divides by `width`/`height` itself.

**Fitting to the judge frame.** The judge renders at a fixed square (e.g. `1024 × 1024`, pinned in `docs/JUDGE.md`). The logical canvas is **scaled-to-fit (contain) and centered** into that frame; margin is filled with the document `background` (or transparent if `null`). Aspect ratio is preserved — never stretch. The exact fit transform is pinned in §10.

> **Scope note for GAME.md / JUDGE.md.** Letterbox bars carry no drawing yet count as judged pixels, so a 16:9 game canvas fit into a square judge frame wastes ~43% of the raster on background — and judged-pixel area affects scores. Choose the canonical **game** canvas aspect ratio (GAME.md) and the **judge** frame aspect ratio (JUDGE.md) *together* (e.g. a square game canvas for a square frame) to minimize letterbox. This is scope guidance for those docs, not a schema change here.

## 3. Top-level `Document`

```ts
/** Bumped on any breaking schema change. v1 = this spec. */
type DocVersion = 1;

interface Document {
  /** MANDATORY, first field. Mirrored to the drawings.doc_version column. */
  version: DocVersion;

  /** Logical canvas size, defines the coordinate space. Positive integers. */
  width: number;
  height: number;

  /** Canvas background. Explicit so the judge raster is deterministic.
   *  null = transparent. Otherwise a hex Color (§5.2), alpha permitted.
   *  Default new docs: "#ffffff". Painted as the bottom-most isolated layer (§10). */
  background: Color | null;

  /** Bottom → top paint order. Index 0 painted first. At least one layer. */
  layers: Layer[];

  /** Optional, additive, non-rendering metadata. Never gate logic on it. */
  meta?: DocumentMeta;
}

interface DocumentMeta {
  generator?: string;        // e.g. "justpaint-editor@0.3.1"
  freehandVersion?: string;  // pinned perfect-freehand version this doc was authored against (§5.3, §9)
  createdAt?: string;        // ISO-8601 (advisory; DB columns are authoritative)
  updatedAt?: string;
  // NOTE: owner/prompt/match identity lives in DB columns + game tables, NOT here.
}
```

- `version` is **first** and **required**. Retrofitting a version tag onto un-versioned blobs is the classic avoidable pain; we pay the one byte now.
- Queryable fields (`owner`, `matchId`, canvas size, version) are **promoted to SQL columns** server-side (§7). The document is *just the picture* plus advisory `meta`.

## 4. `Layer`

```ts
interface Layer {
  id: Id;            // stable, unique within the document
  name: string;      // user-facing label, 1..64 chars
  visible: boolean;  // false ⇒ skipped by renderer AND judge
  opacity: number;   // 0..1, multiplies all strokes on this layer
  strokes: Stroke[]; // ordered; array index = z within the layer (later = on top)
}
```

A layer maps directly to a `Konva.Layer` (its own `<canvas>` — the real-compositing property that won Konva over Fabric). `opacity`/`visible` map to Konva layer attrs. A document with one empty layer renders to just the `background`.

**Erase is per-stroke and layer-isolated.** A `destination-out` stroke erases only content painted *earlier on the same layer* — never the background, never lower layers. This is the single most load-bearing rendering invariant; the headless renderer enforces it by giving each layer its own offscreen surface before compositing (§10). Get layer/stroke iteration order or surface isolation wrong and erases leak across layers. No per-layer blend mode in v1 (seam in §9).

## 5. `Stroke` — discriminated union on `type`

`Stroke` is a tagged union; `type` is the discriminator. The enum is **closed for v1** (the validator rejects unknown types) but extensible by additive version bump. This is the **only** polymorphism in the format: in TS `switch (s.type)`; in Go, decode `{Type string}` first, then re-decode into the concrete struct.

```ts
type StrokeType = "freehand" | "line" | "rect" | "ellipse" | "polygon";

type Stroke =
  | FreehandStroke   // pen / brush  AND  eraser
  | LineStroke       // straight line / open polyline
  | RectStroke       // rectangle
  | EllipseStroke    // ellipse
  | PolygonStroke;   // triangle / closed N-gon

/** Fields shared by every stroke. Kept minimal; style is per-variant. */
interface StrokeBase {
  id: Id;              // stable, unique within the document
  type: StrokeType;    // discriminant
  /** Compositing against earlier content on the SAME layer.
   *  "source-over" = normal paint. "destination-out" = ERASE.
   *  Any stroke type may erase (§5.1); the eraser tool is just freehand+destination-out. */
  composite: Composite;
  /** Optional axis-aligned bounds cache in logical coords (§8). Derived; advisory. */
  bbox?: BBox;
}
```

### 5.1 Tool → stroke mapping (the corrected inventory)

The old engine's six tools, with its three known bugs **fixed in the model**:

| Editor tool | Stroke `type` | Notes / correction vs. old engine |
|---|---|---|
| Pen / Brush | `freehand`, `composite:"source-over"` | perfect-freehand; stores input points + options. |
| Eraser | `freehand`, `composite:"destination-out"` | The old engine already did `destination-out` correctly (`CanvasTool.ts:309-323`); we adopt that behavior, generalized to a per-stroke `composite` field — not a bug fix, a model unification. |
| Line | `line` | Open 2-point polyline. |
| Rectangle | `rect` | Old "Square" was a free rectangle — renamed to its true shape. |
| Ellipse | `ellipse` | Old "Circle" used a `/√2` bbox quirk (`CanvasTool.ts:363-364`) — **dropped**; standard center+radii. |
| Triangle | `polygon` (3 pts) | Old "Triangle" **drew a rect** (`CanvasTool.ts:386-396`) — fixed; a real closed 3-point polygon. |

`polygon` is the general closed-region primitive; a triangle is its 3-point case. A constrained square is a `rect` with `width === height` (editor concern, not a separate type). A perfect circle is `rx === ry`.

**`composite` is shared, not freehand-only.** It lives on `StrokeBase` so any stroke can erase — e.g. a `rect` with `composite:"destination-out"` is a valid rectangular eraser cutting earlier content on its layer. The editor only exposes erase via the freehand brush in v1, but the schema and validator bless `destination-out` on every type (no special-case rejection); the renderer treats it uniformly.

### 5.2 Style primitives

```ts
type Id = string;                  // opaque, stable, unique within the doc; ≤64 chars. Recommend nanoid/ULID.
type Color = string;               // "#rrggbb" or "#rrggbbaa", lowercase hex. ONE format only.
type Composite = "source-over" | "destination-out";
type LineCap  = "butt" | "round" | "square";
type LineJoin = "miter" | "round" | "bevel";
type BBox = { x: number; y: number; width: number; height: number }; // logical coords
```

- **Color: exactly one representation** — lowercase hex `#rrggbb` (opaque) or `#rrggbbaa` (alpha). No `rgb()`, no named colors, no `{r,g,b}`. Go validates with `^#([0-9a-f]{6}|[0-9a-f]{8})$`. The old dev/prod default-color divergence (`#8000ff` vs black, `CanvasTool.ts:69-77`) **must not exist** — defaults live in editor/tool config, identical everywhere; the document always carries explicit resolved colors.
- **Per-stroke transparency in v1 = the `aa` channel of the color.** There is no separate per-stroke `opacity` *field* in v1; a translucent shape carries `#rrggbbaa`. The deferred §9 seam is a distinct *multiplicative* `opacity` (independent of fill/stroke alpha), added only when alpha-in-color no longer suffices. So: alpha gives translucency now; the seam adds a second, orthogonal knob later. (Caveat for freehand below — §5.3.)
- **Fill vs stroke** are independent optional channels on shapes. `null`/absent = that channel is not drawn. This fixes the old "always fill AND stroke" limitation.
- **`composite`** is the only blend control — exactly two values, no `globalCompositeOperation` grab-bag.

### 5.3 `FreehandStroke` (pen / brush / eraser)

**Store input, never the outline.** The renderer runs `getStroke(points, brush)` → outline polygon → fills it. Storing the outline would be 2–4× larger, lossy for re-tuning, version-fragile, and would destroy replay ordering.

```ts
interface FreehandStroke extends StrokeBase {
  type: "freehand";
  /** Brush fill color. For eraser (destination-out) the renderer ignores it (kept for uniformity). */
  color: Color;
  /** Time-ordered input samples, logical coords. pressure ∈ 0..1.
   *  No device pressure ⇒ capture a constant (e.g. 0.5) + brush.simulatePressure. ≥1 point (a dot is valid). */
  points: Array<[x: number, y: number, pressure: number]>;
  /** Curated subset of perfect-freehand getStroke() options (see "Render contract" below). */
  brush: BrushOptions;
}

interface BrushOptions {
  size: number;              // base diameter, logical px.   default 16
  thinning: number;          // -1..1, pressure→width.       default 0.5
  smoothing: number;         // 0..1, outline softening.     default 0.5
  streamline: number;        // 0..1, input low-pass.        default 0.5
  simulatePressure: boolean; // synth pressure from speed.   default true
  taperStart: number;        // logical px, start taper ≥0.  default 0  → maps to getStroke start.taper
  taperEnd: number;          // logical px, end taper ≥0.    default 0  → maps to getStroke end.taper
}
```

**`brush` is a curated subset of `getStroke()` options, NOT the full set.** perfect-freehand also accepts `easing`, `last`, and per-end `cap`/`taper`/`easing` under nested `start`/`end`. We deliberately store only the seven fields above and **pin the omitted options as fixed renderer constants** that every consumer (editor preview + server worker) passes identically:

| Omitted getStroke option | Pinned value (not stored) |
|---|---|
| `start.cap`, `end.cap` | `true` (rounded caps) |
| `start.easing`, `end.easing` | linear (`t => t`) |
| `easing` (global pressure→thinning) | linear (`t => t`) |
| `last` | `true` (final point is the true end) |

Mapping into `getStroke`: `taperStart → start.taper`, `taperEnd → end.taper`. The editor **must not** expose a control that varies a pinned option without first adding it to `BrushOptions` and bumping the render contract — otherwise editor and worker diverge silently. If a future cap/easing control is wanted, add explicit fields (e.g. `capStart`/`capEnd`) as a §9 additive seam.

**Render:** `outline = getStroke(points, brush)` → `Konva.Line { points: flatten(outline), closed: true, fill: color, strokeWidth: 0, globalCompositeOperation: composite }`. The outline is *filled as a straight-segment polygon* — **do not** route it through perfect-freehand's `getSvgPathFromStroke` curve smoothing; both surfaces fill the same polyline so they stay byte-aligned (§6).

**Translucent freehand is not supported in v1.** The freehand outline is a single self-overlapping filled polygon (the ribbon doubles back on itself), so a `color` with `aa < ff` produces darkened seams where the fill overlaps — a non-uniform artifact. Treat brush alpha as effectively opaque in v1; uniform per-stroke freehand opacity is the §9 seam. Shapes (`rect`/`ellipse`/`polygon`/`line`) have well-defined alpha and may use it freely.

**Determinism contract.** `getStroke` output is a pure function of `(points, brush, pinned options, perfect-freehand version)`. Therefore the **pinned perfect-freehand version is part of the render contract** — pinned in the monorepo lockfile, recorded in `meta.freehandVersion`, and identical across editor preview and the server render worker. The judge **never runs `getStroke`**: it consumes pre-rendered PNGs (§10), which deletes the cross-language determinism problem from the ML boundary.

### 5.4 `LineStroke` (straight line / open polyline)

```ts
interface LineStroke extends StrokeBase {
  type: "line";
  points: Array<[x: number, y: number]>; // ≥2 points (2-tuples)
  stroke: Color;
  strokeWidth: number;                   // logical px, >0
  cap?: LineCap;                         // default "round"
  join?: LineJoin;                       // default "round"
}
```

The **Line tool** is exactly two points. No `tension`, `dash`, or `closed` in v1 — freehand owns smoothing; closed filled shapes are `polygon` (additive seams in §9). Maps to `Konva.Line` with `tension: 0`.

### 5.5 `RectStroke` (rectangle)

```ts
interface RectStroke extends StrokeBase {
  type: "rect";
  x: number; y: number;          // top-left, logical
  width: number; height: number; // logical, ≥0 (normalize negative drags on commit)
  cornerRadius?: number;         // logical px ≥0, default 0
  fill?: Color | null;           // null/absent = no fill
  stroke?: Color | null;         // null/absent = no outline
  strokeWidth?: number;          // logical px, default 1; >0 when stroke present; ignored if no stroke
}
```

Maps to `Konva.Rect`. **Normalize at creation:** a drag from bottom-right to top-left is stored with non-negative `width/height` and corrected top-left `x/y`. Negative extents are invalid (keeps renderer and bbox trivial). A zero-area rect (`width === 0` or `height === 0`) is **degenerate and rejected** by the validator — commit only rects with positive extent.

### 5.6 `EllipseStroke` (ellipse)

```ts
interface EllipseStroke extends StrokeBase {
  type: "ellipse";
  cx: number; cy: number;        // CENTER, logical (matches Konva.Ellipse x/y)
  rx: number; ry: number;        // radii, logical, >0
  fill?: Color | null;
  stroke?: Color | null;
  strokeWidth?: number;          // default 1; >0 when stroke present
}
```

Maps to `Konva.Ellipse`. Created from the drag bbox `(x,y,w,h)`: `cx = x + w/2`, `cy = y + h/2`, `rx = |w|/2`, `ry = |h|/2`. The old `/√2` quirk is gone.

### 5.7 `PolygonStroke` (triangle / closed N-gon)

```ts
interface PolygonStroke extends StrokeBase {
  type: "polygon";
  points: Array<[x: number, y: number]>; // ≥3 vertices (2-tuples); implicitly closed
  fill?: Color | null;
  stroke?: Color | null;
  strokeWidth?: number;                  // default 1; >0 when stroke present
  join?: LineJoin;                       // default "round"
}
```

Renders as `Konva.Line { closed: true }`. A **triangle** is created from the drag bbox as three vertices (apex-top by convention: `[cx, y]`, `[x+w, y+h]`, `[x, y+h]`) — a **real** triangle, fixing the old rectangle bug. A `polygon` is a *filled region*; a closed `line` would be a stroked path — keeping them distinct keeps intent and validation clean (polygon requires ≥3 vertices and supports `fill`).

## 6. Konva mapping (render + hit-test)

The schema is a **projection** of Konva, never Konva's serialization. At load: `document → Konva nodes`. At edit: `Konva change → document command` (§10).

We never persist `Stage.toJSON()`. (`DECISIONS.md` lists Konva's JSON serialization as a reason it beat Fabric — that convenience is real, but we use it only as a **dev-only debug dump**, never as the wire/storage format: it vendor-locks to Konva class names + version, drops attrs, stores rendered centerlines instead of intent, and has no version/history/match metadata. Our document is the canonical schema; Konva's JSON is a throwaway diagnostic.)

| Document | Konva | Field mapping |
|---|---|---|
| `Document` | `Konva.Stage` | `width`, `height` (logical); `background` → bottom isolated layer (§10) |
| `Layer` | `Konva.Layer` | own `<canvas>`; `opacity`, `visible` |
| `freehand` | `Konva.Line` | `points = flatten(getStroke(points, brush))`; `closed:true`; `fill:color`; `strokeWidth:0`; straight-segment fill (no SVG-curve smoothing) |
| `line` | `Konva.Line` | `points` (flattened); `stroke`, `strokeWidth`, `lineCap=cap`, `lineJoin=join`, `tension:0` |
| `rect` | `Konva.Rect` | `x,y,width,height,cornerRadius,fill,stroke,strokeWidth` |
| `ellipse` | `Konva.Ellipse` | `x=cx`, `y=cy`, `radiusX=rx`, `radiusY=ry`, `fill,stroke,strokeWidth` |
| `polygon` | `Konva.Line` | `points` (flattened); `closed:true`; `fill,stroke,strokeWidth,lineJoin=join` |
| any | shape attrs | `composite` → `globalCompositeOperation`; `id` → `id` |

There is no native `Konva.Triangle`; triangle = `Konva.Line` with 3 points + `closed:true`, so the old bug is structurally impossible. Hit-testing uses Konva's built-in hit graph on the projected shapes; node `id` ties a Konva shape back to its document stroke for selection/edit.

## 7. jsonb storage & Go validation

Keep the document **opaque at the DB layer** — store the bytes, promote anything you query/sort on into real columns. **This DDL is authoritative for the `drawings` table; `docs/ARCHITECTURE.md` references it rather than re-declaring it.**

```sql
create table drawings (
  id            uuid primary key,
  owner_id      uuid not null references users(id),
  match_id      uuid null references matches(id),
  doc_version   int  not null,          -- mirror of document.version (cheap filter/migrate)
  width         int  not null,          -- promoted from jsonb
  height        int  not null,
  document      jsonb not null,         -- the vector document, opaque to SQL
  thumbnail_url text null,              -- cached preview PNG in object storage (derived artifact; never the source of truth)
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now()
);
```

- **pgx v5:** bind `json.RawMessage` / `[]byte` straight to `jsonb` (no custom type).
- **sqlc:** override the `document` column's Go type so it emits `json.RawMessage`, not `interface{}`/`pgtype.JSONB`:
  ```yaml
  overrides:
    - column: "drawings.document"
      go_type: "encoding/json.RawMessage"
  ```
- Promote `owner_id`, `match_id`, `doc_version`, `width`, `height` to columns; leave layers/strokes in jsonb. `thumbnail_url` points at a rendered PNG in object storage (consistent with "rendered PNGs to object storage"); it is a cache, never authoritative. No GIN index needed — documents are fetched whole by id.
- Postgres validates JSON well-formedness only, not shape. Shape validation is a Go concern.

**Validation in the Go handler, at the write edge** — light but strict on what the renderer/judge depend on:

1. **Size guard first.** `http.MaxBytesReader` caps the body before parsing (exact bytes pinned in `docs/API.md`). A multi-million-point jsonb would OOM the judge's rasterizer — a DoS boundary, not a nicety.
2. **Decode into typed structs** mirroring §3–§5 via a `Stroke.UnmarshalJSON` that decodes `{Type string}` then re-decodes the concrete struct. **Allow unknown fields** — do *not* use `DisallowUnknownFields` (forward-compat: older servers must tolerate newer additive fields).
3. **Nullable Color fields must be pointers in Go.** `fill`, `stroke`, and `background` are tri-state — *absent* / `null` / a value — and absent-or-null means "do not draw this channel". A plain `string` cannot distinguish them; use `*Color` (pointer). Apply `cornerRadius`/`strokeWidth`/cap/join defaults post-decode.
4. **Enforce invariants:**
   - known `version`; `1 ≤ width,height ≤ 8192`; ≥1 layer.
   - `color` matches the hex regex; `composite`/`type`/`cap`/`join` in their enums.
   - `opacity` (layer) and `pressure` (freehand point) ∈ `[0,1]`; every number finite.
   - `size`/radii/`cornerRadius`/tapers ≥0; ellipse `rx,ry > 0`; rect `width,height > 0` (reject zero-area); **`strokeWidth > 0` whenever the `stroke` channel is present** (use `null`/absent to omit an outline, not `0`).
   - **point arity:** freehand `points` are **3-tuples** `[x,y,pressure]`, ≥1; line `points` are **2-tuples**, ≥2; polygon `points` are **2-tuples**, ≥3. Reject mismatched arity.
   - `id` non-empty, ≤64 chars, **unique within the document** — walk all layer ids and all stroke ids into one set (single namespace).
5. **DoS caps** (exact numbers pinned in `docs/API.md`). The **binding limit is total points** — that budget always trips before any reasonable per-layer stroke count, so size the validator around it: a global cap on **total input points** plus a cap on **total strokes** and **layers**. Don't advertise a per-layer stroke cap the point budget makes unreachable.
6. **Set `doc_version`, `width`, `height` columns** from the validated doc.

The canonical schema is the TS in `packages/document`; the Go validator mirrors it by hand for v1. A shared **JSON Schema** generated from the TS types is the clean way to keep both honest later. Both sides validate against *the spec*, not against each other's code.

## 8. `id` and `bbox`

- **`id`** — every `Layer` and `Stroke` carries a stable, document-unique string id. The format treats it as opaque (any non-empty string ≤64 chars). Recommend nanoid or ULID (sortable, collision-safe, client-generatable for offline/optimistic edits). Required for command-based undo/redo and future realtime conflict resolution; the editor assigns on create and never reuses. Uniqueness is per-document across layers and strokes (one namespace).
- **`bbox`** — optional, **derived** axis-aligned bounds in logical coords, cached for fast hit-testing / dirty-region redraw / culling. **Never authoritative:** if it disagrees with geometry, geometry wins; the renderer may recompute and validators must not trust it. For freehand the cache should bound the *rendered outline* (include `brush.size`/taper), not just raw input points; an approximate point-AABB is acceptable. Writers may omit it; the server may strip it to shrink jsonb.

## 9. Versioning & forward-compatible seams

- **`version`** is a mandatory monotonic integer, first field, mirrored to `doc_version`. v1 = this spec. Greenfield: we start at `1` with zero legacy, but the field and upcaster seam exist from day one.
- **Additive changes do NOT bump it:** new optional fields, new `meta` keys, populating `bbox`. Consumers ignore unknown fields. This is the default evolution path — most growth lands here.
- **Breaking changes bump it:** removing/renaming a field, changing units/semantics, adding/removing a `StrokeType`, changing the coordinate model, changing a pinned brush/render constant in a way that alters geometry. On bump, write an upcaster `vN → vN+1` in `packages/document`; the read path upcasts lazily in memory. For bulk rewrites a goose migration walks the jsonb column (rows found cheaply via the `doc_version` column without parsing).
- **Render contract = `version` + pinned perfect-freehand version + the pinned brush/fill/fit constants (§5.3, §6, §10).** All recorded or pinned in the lockfile; a brush-engine upgrade or a fit-math change that alters geometry is a render-contract change, coordinated across editor + worker and triggering re-render of cached PNGs.

**v1 replay scope.** Goal #4 ("faithful replay") is **order-based** and bounded: reveal strokes sequentially in array order; freehand may sub-reveal its points in capture order; shapes appear **atomically** (no intra-shape animation); inter-stroke timing is a presentation choice (there are no timestamps in v1). True timed playback is a §9 seam below — don't expect it from a v1 doc.

**Seams already in place (so future features are additive, version-safe):**
- `meta` — arbitrary advisory metadata.
- `StrokeBase.bbox` — perf cache, addable/removable freely.
- Per-stroke *multiplicative* `opacity` — independent of color alpha; for uniform translucent strokes (and the well-defined way to get translucent freehand, §5.3).
- Freehand `capStart`/`capEnd` (and easing) — when the editor wants to vary the currently-pinned cap/easing constants.
- `line.tension` / `line.dash` / `line.closed` — smoothed/dashed/closed lines.
- Per-layer `blend` and per-stroke blend modes beyond the two `composite` values.
- Optional per-point timestamps or per-stroke `t0`/`dt` for **true timed replay**.
- Shared `transform?: { x, y, rotation, scaleX, scaleY }` on shapes, mirroring `Konva.Transform`, once a move/rotate tool exists. Deliberately **off freehand** to keep `points` authoritative.
- Text / image / gradient nodes.

## 10. Serialize / deserialize / render → PNG

**Canonical = this schema.** `packages/document` owns serialize/deserialize plus **one renderer** used in three places (editor preview, authoritative server raster, and thus the judge image) so they are byte-aligned by construction. Indicative API:

```ts
function parseDocument(json: unknown): Document;             // validate + upcast (§7, §9)
function serializeDocument(doc: Document): string;           // canonical jsonb payload
function toKonva(doc: Document, Konva): Konva.Stage;         // projection for editor/preview
function renderToPNG(doc: Document, opts: RenderOptions): Uint8Array;

interface RenderOptions {
  outWidth: number; outHeight: number;   // FINAL PNG pixel dimensions (e.g. judge frame 1024×1024)
  fit: "contain";                        // scale-to-fit + center (below); "stretch" reserved
  background?: Color | null;             // REPLACES doc.background (e.g. force opaque white for the judge)
  pixelRatio?: number;                   // internal supersample factor ONLY; never changes output dims
}
```

**`pixelRatio` does not change output size.** `outWidth`/`outHeight` are the exact final PNG dimensions. `pixelRatio` is an internal anti-aliasing supersample factor: render at `outWidth*pixelRatio × outHeight*pixelRatio`, then downscale to `outWidth × outHeight`. The judge frame must be exactly what `docs/JUDGE.md` pins, so an accidental `pixelRatio` must never double it. (On the editor's browser path, `pixelRatio` may map directly to `stage.toDataURL({ pixelRatio })` for crisp previews — that path sets its own dimensions.)

**Pinned fit transform (contain).** Identical sub-pixel math on every renderer, or anti-aliased edges shift and judge pixels change:

```
scale = min(outWidth / width, outHeight / height)
dx    = (outWidth  - width  * scale) / 2
dy    = (outHeight - height * scale) / 2
```

Apply `translate(dx, dy)` then `scale(scale)`; **do not round `dx`/`dy`** (keep them fractional and let the rasterizer anti-alias the offset consistently). This, plus the brush/fill pins (§5.3, §6), is part of the render contract.

**Render algorithm (deterministic, per-layer isolation is mandatory):**

1. Allocate the output surface at `outWidth*pixelRatio × outHeight*pixelRatio`.
2. Compute the effective background: `RenderOptions.background` if provided, else `document.background`. If non-null, paint it as the **bottom-most fully-isolated layer** (its own surface, composited under everything) so it is **never an erase target** — a full-canvas `destination-out` stroke must not punch through to transparency. If null, leave transparent.
3. Apply the pinned fit transform (above) so the logical canvas is contained + centered.
4. For each `layer` in order (index 0 first), skip if `!visible`:
   - Render the layer into **its own transparent offscreen surface**. Strokes composite (`source-over` / `destination-out`) against *only that surface*, so `destination-out` erases only earlier strokes on the **same layer** — never the background, never lower layers. This matches `Konva.Layer` semantics exactly, keeping editor and worker byte-aligned.
   - Multiply the finished layer surface by `layer.opacity` and composite it (`source-over`) onto the document accumulator.
5. Downscale by `pixelRatio` and flatten → PNG.

**Two surfaces, one renderer:**
- **Editor (browser, Konva):** `toKonva` → `stage.toDataURL({ pixelRatio, width, height })` for previews/thumbnails. Konva's own per-layer `<canvas>` gives the same per-layer isolation the algorithm mandates.
- **Server/judge (headless):** a **Node render worker** importing `packages/document` + Konva-node/`node-canvas` + the pinned perfect-freehand, emitting the fixed-size PNG, following the per-layer-isolation algorithm above. A Go-native rasterizer is a *trap* — a second renderer to keep pixel-identical to Konva+perfect-freehand. Avoid it.

**Trust boundary (game-critical):**
- The client submits the **vector document, never a scored PNG.** A client thumbnail may ride along for instant UI — **advisory only**; a cheater could doctor it.
- The judged raster is rendered **authoritatively off the player's machine** from the document.
- **The judge receives pre-rendered PNGs**, never the document. The contract stays `(prompt, pngA, pngB) → { scoreA, scoreB, winner, reason }` (per `docs/DECISIONS.md`; the exact `winner` type and A/B→player mapping are pinned in `docs/JUDGE.md`). The ML collaborator never parses our schema or runs `getStroke`. PNGs go to object storage; the judge gets URLs/bytes.

**Undo/redo & replay** are runtime, not persisted. The editor maintains the canonical `Document` directly; Konva edits emit **commands** (add/remove/update/reorder stroke, layer ops) that mutate it, keyed by `id`. Undo/redo is a command stack over the document — **not** PNG snapshots (the old `CanvasHistory.ts` PNG stack and `bytea`/base64-per-layer storage are dropped entirely). The command stack is **never persisted in jsonb**; the saved document is the flattened current state, already sufficient for order-based replay.

Open items consumed by (not changed by) this format, pinned elsewhere: judge resolution + transparent-vs-white background + the `winner` type → `docs/JUDGE.md`; game canonical canvas size (chosen to minimize judge letterbox, §2) → `docs/GAME.md`; body-size cap + per-doc limits → `docs/API.md`.

## 11. Worked examples

### 11.1 Minimal — one pen stroke on white

```json
{
  "version": 1,
  "width": 1920,
  "height": 1080,
  "background": "#ffffff",
  "layers": [
    {
      "id": "lyr_base",
      "name": "Layer 1",
      "visible": true,
      "opacity": 1,
      "strokes": [
        {
          "id": "stk_pen01",
          "type": "freehand",
          "composite": "source-over",
          "color": "#1b1b1b",
          "points": [[420, 300, 0.42], [432.5, 305.1, 0.55], [450, 312, 0.61], [470, 320, 0.4]],
          "brush": {
            "size": 16, "thinning": 0.5, "smoothing": 0.5,
            "streamline": 0.5, "simulatePressure": true,
            "taperStart": 0, "taperEnd": 0
          }
        }
      ]
    }
  ],
  "meta": { "generator": "justpaint-editor@0.3.1", "freehandVersion": "1.2.4" }
}
```

### 11.2 Full tool set across two layers, with an eraser

```json
{
  "version": 1,
  "width": 1920,
  "height": 1080,
  "background": null,
  "layers": [
    {
      "id": "lyr_shapes",
      "name": "Shapes",
      "visible": true,
      "opacity": 1,
      "strokes": [
        {
          "id": "stk_rect", "type": "rect", "composite": "source-over",
          "x": 200, "y": 150, "width": 400, "height": 260, "cornerRadius": 12,
          "fill": "#cfe8ff", "stroke": "#0050a0", "strokeWidth": 3
        },
        {
          "id": "stk_ell", "type": "ellipse", "composite": "source-over",
          "cx": 1100, "cy": 400, "rx": 180, "ry": 120,
          "fill": "#ffe0b3", "stroke": null
        },
        {
          "id": "stk_tri", "type": "polygon", "composite": "source-over",
          "points": [[800, 700], [950, 950], [650, 950]],
          "fill": "#d9ffd9", "stroke": "#1b7a1b", "strokeWidth": 2, "join": "round"
        },
        {
          "id": "stk_line", "type": "line", "composite": "source-over",
          "points": [[100, 1000], [1820, 1000]],
          "stroke": "#333333", "strokeWidth": 4, "cap": "round", "join": "round"
        }
      ]
    },
    {
      "id": "lyr_ink",
      "name": "Ink",
      "visible": true,
      "opacity": 0.85,
      "strokes": [
        {
          "id": "stk_pen", "type": "freehand", "composite": "source-over",
          "color": "#1b1b1b",
          "points": [[300, 200, 0.4], [340, 230, 0.6], [400, 250, 0.7], [470, 240, 0.5]],
          "brush": {
            "size": 22, "thinning": 0.6, "smoothing": 0.5,
            "streamline": 0.6, "simulatePressure": true,
            "taperStart": 0, "taperEnd": 12
          }
        },
        {
          "id": "stk_erase", "type": "freehand", "composite": "destination-out",
          "color": "#000000",
          "points": [[380, 235, 0.9], [410, 245, 0.9]],
          "brush": {
            "size": 30, "thinning": 0, "smoothing": 0.5,
            "streamline": 0.4, "simulatePressure": false,
            "taperStart": 0, "taperEnd": 0
          }
        }
      ]
    }
  ],
  "meta": { "generator": "justpaint-editor@0.3.1", "freehandVersion": "1.2.4" }
}
```

`background: null` (transparent); the `Ink` layer is 85% opaque; the eraser (`destination-out`) cuts only the pen stroke **on the same `Ink` layer** — `Shapes` is untouched; the triangle is a real closed 3-point polygon. The eraser's `color` is ignored by the renderer (kept for schema uniformity).

## 12. Opinionated calls (what we bought, deferred, rejected)

- **Bought:** mandatory `version` + DB mirror; per-object `id`s; store-input-not-output for freehand; explicit logical coordinate space + explicit `background`; per-stroke `composite` for erase (any type); pinned brush subset + fit/fill math as the render contract; per-layer surface isolation in the headless renderer; `render→PNG` as a first-class `packages/document` capability; pre-rendered PNGs to the judge; one renderer shared by editor/server/judge.
- **Deferred (with seams, §9):** per-stroke multiplicative opacity (incl. translucent freehand); freehand cap/easing controls; `line` tension/dash/closed; blend modes; shape `transform` for post-creation editing; timed replay; text/images/gradients.
- **Rejected:** persisting Konva `Stage.toJSON()` as storage; PNG-snapshot history; `bytea`/base64-PNG-per-layer; per-layer coordinate spaces; normalized `0..1` coords; multiple color formats; `DisallowUnknownFields`; a Go-native second renderer; storing the full perfect-freehand option set (curated subset + pinned constants instead).
