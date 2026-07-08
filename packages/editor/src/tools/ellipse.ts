// packages/editor/src/tools/ellipse.ts — the ellipse tool (DOCUMENT-FORMAT §5.6).
import type { EllipseStroke } from "@justpaint/document";
import type { StrokeTool } from "../types";

/**
 * Ellipse tool: a PURE transformer from a drag gesture to a single
 * {@link EllipseStroke}. The ellipse is the axis-aligned bbox of the drag's
 * first and last points (§5.6):
 *
 *   cx = x + w/2,  cy = y + h/2,  rx = |w|/2,  ry = |h|/2
 *
 * No `/√2` quirk (the old engine's bug is dropped). Intermediate gesture points
 * are ignored — only the down/up corners define the bbox. Returns `null` for a
 * degenerate gesture (rx ≤ 0 or ry ≤ 0), which the editor discards.
 */
export const ellipseTool: StrokeTool = {
  kind: "stroke",
  id: "ellipse",
  buildStroke(ctx, gesture) {
    const first = gesture[0];
    const last = gesture[gesture.length - 1];
    if (first === undefined || last === undefined) return null;

    // Drag bbox from the two corners (w/h may be negative; radii take |·|).
    const w = last.x - first.x;
    const h = last.y - first.y;

    const cx = first.x + w / 2;
    const cy = first.y + h / 2;
    const rx = Math.abs(w) / 2;
    const ry = Math.abs(h) / 2;

    // Degenerate: a zero-extent drag in either axis has no ellipse to draw.
    if (rx <= 0 || ry <= 0) return null;

    const stroke: EllipseStroke = {
      id: ctx.newId(),
      type: "ellipse",
      composite: "source-over",
      cx,
      cy,
      rx,
      ry,
      fill: ctx.style.fill,
      stroke: ctx.style.color,
      strokeWidth: ctx.style.strokeWidth,
    };
    return stroke;
  },
};
