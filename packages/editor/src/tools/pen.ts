import type { FreehandStroke } from "@justpaint/document";
import type { LogicalPoint, StrokeTool, ToolContext } from "../types";

/**
 * Pen / brush — the freehand tool (DOCUMENT-FORMAT §5.3).
 *
 * Pure: a drag gesture → one `freehand` stroke, `composite: "source-over"`.
 * Stores raw input points (`[x, y, pressure]`), never the rendered outline —
 * the renderer runs `getStroke(points, brush)` later. Rounds nothing here;
 * serialization handles write-precision (§2).
 *
 * NEVER returns null: a single sample is a valid dot (a freehand stroke needs
 * ≥1 point). The only way the editor avoids a stroke is by not committing a
 * zero-sample gesture, which the editor never produces.
 */
export const penTool: StrokeTool = {
  kind: "stroke",
  id: "pen",
  buildStroke(ctx: ToolContext, gesture: readonly LogicalPoint[]): FreehandStroke | null {
    if (gesture.length < 1) return null;

    return {
      id: ctx.newId(),
      type: "freehand",
      composite: "source-over",
      color: ctx.style.color,
      points: gesture.map((p) => [p.x, p.y, p.pressure]),
      brush: ctx.style.brush,
    };
  },
};
