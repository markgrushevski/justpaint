// packages/editor/src/types.ts — the FROZEN contract every tool implements.
import type { BrushOptions, Stroke } from "@justpaint/document";

/** Ids of the stroke-producing drawing tools (see {@link StrokeTool}). */
export type StrokeToolId = "pen" | "eraser" | "line" | "rect" | "ellipse" | "triangle";

/** Every selectable tool: the stroke tools plus the pan-only hand. */
export type ToolId = StrokeToolId | "hand";

/** A pointer sample in LOGICAL document coordinates. pressure in [0,1]. */
export interface LogicalPoint { x: number; y: number; pressure: number; }

/**
 * A read-only snapshot of a document layer for the editor's host UI (a layers
 * panel). The editor owns the mutable {@link Layer}; the host renders these and
 * calls back into the editor to mutate. Ordered bottom→top, matching the
 * document's z-order (`layers[0]` is the bottom-most).
 */
export interface LayerView {
  readonly id: string;
  readonly name: string;
  readonly visible: boolean;
  readonly opacity: number;
  readonly strokeCount: number;
}

/** Current drawing style the editor supplies to tools. */
export interface ToolStyle {
  color: string;        // stroke/brush color, "#rrggbb" or "#rrggbbaa"
  fill: string | null;  // shape fill, or null for no fill
  strokeWidth: number;  // shape/line stroke width, > 0
  brush: BrushOptions;  // freehand brush options
}

/** Services a tool needs from the editor. */
export interface ToolContext {
  readonly style: ToolStyle;
  newId(): string;      // returns a fresh document-unique id
}

/**
 * A drawing tool: a PURE transformer from a drag gesture (ordered logical
 * points; first = pointerdown, last = current/pointerup) to a single Stroke.
 * No internal mutable state, no Konva imports — unit-testable in isolation.
 * Returns null for a degenerate gesture that must be discarded.
 */
export interface StrokeTool {
  readonly kind: "stroke";
  readonly id: StrokeToolId;
  buildStroke(ctx: ToolContext, gesture: readonly LogicalPoint[]): Stroke | null;
}

/**
 * A view-manipulation tool: no `buildStroke` AT ALL — the type system, not a
 * runtime check, guarantees it can never touch the document or the history.
 * The editor routes its pointerdowns to the pan path (the same mechanics as a
 * middle-button drag) BEFORE the stroke-gesture pipeline, skipping the
 * inside-document gate (you can grab the letterbox).
 */
export interface PanTool {
  readonly kind: "pan";
  readonly id: "hand";
}

/**
 * Any selectable tool. Discriminated on `kind`: the editor narrows once at
 * pointerdown — `"stroke"` enters the gesture→stroke pipeline, `"pan"` drags
 * the view. New non-stroke tools add a `kind`, not an id-list check.
 */
export type Tool = StrokeTool | PanTool;
