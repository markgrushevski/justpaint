/**
 * Canonical TypeScript types for the justpaint vector document (v1).
 *
 * This is the source of truth that the Go server (`server/internal/document`)
 * mirrors by hand. See `docs/DOCUMENT-FORMAT.md` — when code and that doc
 * disagree, the doc wins until amended there. Both sides validate against the
 * spec, not against each other's code.
 */

/** Bumped only on a breaking schema change. v1 = the current spec. */
export type DocVersion = 1;

/** Opaque, stable, document-unique id (≤64 chars). Recommend nanoid / ULID. */
export type Id = string;

/** Lowercase hex, `#rrggbb` or `#rrggbbaa`. The ONE color representation. */
export type Color = string;

/** Per-stroke blend against earlier content on the SAME layer. */
export type Composite = "source-over" | "destination-out";

export type LineCap = "butt" | "round" | "square";
export type LineJoin = "miter" | "round" | "bevel";

export type StrokeType = "freehand" | "line" | "rect" | "ellipse" | "polygon";

/** `[x, y, pressure]` — logical coords; pressure ∈ [0,1]. */
export type FreehandPoint = [x: number, y: number, pressure: number];

/** `[x, y]` — logical coords. */
export type Point = [x: number, y: number];

/** Optional, derived axis-aligned bounds cache (advisory; never authoritative). */
export interface BBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

/** The canonical vector drawing (docs/DOCUMENT-FORMAT.md §3). */
export interface Document {
  /** MANDATORY, first field. Mirrored to the `drawings.doc_version` column. */
  version: DocVersion;
  /** Logical canvas size; defines the coordinate space. Positive integers. */
  width: number;
  height: number;
  /** `null` = transparent; otherwise a hex color. Painted as the bottom-most
   *  isolated layer so a full-canvas erase can't punch through it. */
  background: Color | null;
  /** Bottom → top paint order; index 0 painted first. At least one layer. */
  layers: Layer[];
  /** Optional, additive, non-rendering metadata. Never gate logic on it. */
  meta?: DocumentMeta;
}

export interface DocumentMeta {
  generator?: string;
  /** Pinned perfect-freehand version this doc was authored against (§5.3, §9). */
  freehandVersion?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface Layer {
  id: Id;
  /** User-facing label, 1..64 chars. */
  name: string;
  /** `false` ⇒ skipped by the renderer AND the judge. */
  visible: boolean;
  /** 0..1, multiplies every stroke on this layer. */
  opacity: number;
  /** Ordered; array index = z within the layer (later = on top). */
  strokes: Stroke[];
}

/** Fields shared by every stroke. Style is per-variant. */
export interface StrokeBase {
  id: Id;
  type: StrokeType;
  /** `source-over` = normal paint; `destination-out` = erase (any stroke type). */
  composite: Composite;
  /** Optional derived bounds cache (§8). Advisory; the renderer may recompute. */
  bbox?: BBox;
}

/** Pen / brush / eraser — perfect-freehand input (store input, never the outline). */
export interface FreehandStroke extends StrokeBase {
  type: "freehand";
  /** Brush fill color. Ignored by the renderer when `composite` is erase. */
  color: Color;
  /** Time-ordered input samples; ≥1 point (a dot is valid). */
  points: FreehandPoint[];
  brush: BrushOptions;
}

/** Curated subset of perfect-freehand `getStroke()` options (see freehand.ts). */
export interface BrushOptions {
  /** Base diameter, logical px. */
  size: number;
  /** -1..1, pressure → width. */
  thinning: number;
  /** 0..1, outline softening. */
  smoothing: number;
  /** 0..1, input low-pass. */
  streamline: number;
  /** Synthesize pressure from speed. */
  simulatePressure: boolean;
  /** Start taper, logical px, ≥0. */
  taperStart: number;
  /** End taper, logical px, ≥0. */
  taperEnd: number;
}

/** Straight line / open polyline (≥2 points). */
export interface LineStroke extends StrokeBase {
  type: "line";
  points: Point[];
  stroke: Color;
  /** Logical px, > 0. */
  strokeWidth: number;
  cap?: LineCap;
  join?: LineJoin;
}

/** Rectangle (top-left + size). */
export interface RectStroke extends StrokeBase {
  type: "rect";
  x: number;
  y: number;
  /** Logical, > 0 (zero-area rejected; normalize negative drags on commit). */
  width: number;
  height: number;
  cornerRadius?: number;
  /** `null`/absent = no fill. */
  fill?: Color | null;
  /** `null`/absent = no outline. */
  stroke?: Color | null;
  /** Default 1; > 0 when `stroke` present; ignored if no stroke. */
  strokeWidth?: number;
}

/** Ellipse (center + radii). */
export interface EllipseStroke extends StrokeBase {
  type: "ellipse";
  cx: number;
  cy: number;
  /** Radii, logical, > 0. */
  rx: number;
  ry: number;
  fill?: Color | null;
  stroke?: Color | null;
  strokeWidth?: number;
}

/** Closed N-gon (triangle = 3 points); implicitly closed. */
export interface PolygonStroke extends StrokeBase {
  type: "polygon";
  /** ≥3 vertices. */
  points: Point[];
  fill?: Color | null;
  stroke?: Color | null;
  strokeWidth?: number;
  join?: LineJoin;
}

/** The discriminated union on `type`. Closed for v1. */
export type Stroke =
  | FreehandStroke
  | LineStroke
  | RectStroke
  | EllipseStroke
  | PolygonStroke;

// --- AI Assist ops (docs/ASSIST.md §2) ---
// A derived, additive contract over Stroke/Layer: what the LLM is allowed to say.
// The Op schema adds no new stroke invariants — it composes the existing ones —
// but narrows the stroke subset (freehand excluded) and lives in both validators
// 1:1, exactly like the Stroke contract.

/** Op-eligible stroke-type subset. Freehand is excluded from AI ops (§2). */
export type OpStrokeType = "line" | "rect" | "ellipse" | "polygon";

/** Op-eligible stroke subset — the non-freehand shapes. */
export type OpStroke = LineStroke | RectStroke | EllipseStroke | PolygonStroke;

/**
 * One AI-assist operation over the document. Discriminated on `kind`; closed for
 * Phase A (`update_stroke`/`delete_stroke` are v2, ASSIST.md §2). camelCase on the
 * wire. `add_layer.id` is LLM-assigned and validated in the same single id
 * namespace as document layers/strokes; `add_stroke.layerId` must resolve to a
 * summary layer or an earlier `add_layer` in the same batch.
 */
export type Op =
  | { kind: "add_layer"; id: Id; name: string }
  | { kind: "add_stroke"; layerId: Id; stroke: OpStroke };

/**
 * Minimal document summary the assist endpoint receives (Phase A). Just enough to
 * seed the id namespace and resolve layer references — NOT the full document.
 * See docs/DESIGN-ASSIST-PHASE-A.md §1 resolution 3.
 */
export interface DocSummary {
  canvas: { width: number; height: number };
  layers: Array<{ id: Id; name: string; strokeCount: number }>;
}
