// packages/editor/src/types.ts — the FROZEN contract every tool implements.
import type { BrushOptions, Stroke } from "@justpaint/document";

export type ToolId = "pen" | "eraser" | "line" | "rect" | "ellipse" | "triangle";

/** A pointer sample in LOGICAL document coordinates. pressure in [0,1]. */
export interface LogicalPoint { x: number; y: number; pressure: number; }

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
export interface Tool {
  readonly id: ToolId;
  buildStroke(ctx: ToolContext, gesture: readonly LogicalPoint[]): Stroke | null;
}
