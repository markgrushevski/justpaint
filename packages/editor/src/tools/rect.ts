import type { RectStroke } from "@justpaint/document";
import type { StrokeTool } from "../types";

/**
 * Rectangle tool. Builds a {@link RectStroke} from the axis-aligned bounding
 * box of the gesture's first (pointerdown) and last (current/pointerup) points.
 *
 * PURE: no Konva, no module state, no side effects — everything is derived from
 * `ctx` + `gesture`. Negative drags (bottom-right → top-left) are normalized to
 * a non-negative width/height with a corrected top-left x/y, per
 * docs/DOCUMENT-FORMAT.md §5.5. Returns `null` for a zero-area (degenerate)
 * rect, which the validator would reject anyway.
 */
export const rectTool: StrokeTool = {
  kind: "stroke",
  id: "rect",
  buildStroke(ctx, gesture) {
    const a = gesture[0];
    const b = gesture[gesture.length - 1];
    if (a === undefined || b === undefined) return null;

    // Bounding box of the two corners, normalized to non-negative extents.
    const x = Math.min(a.x, b.x);
    const y = Math.min(a.y, b.y);
    const width = Math.abs(b.x - a.x);
    const height = Math.abs(b.y - a.y);

    // Degenerate: zero area. The document validator rejects these.
    if (width <= 0 || height <= 0) return null;

    const stroke: RectStroke = {
      id: ctx.newId(),
      type: "rect",
      composite: "source-over",
      x,
      y,
      width,
      height,
      fill: ctx.style.fill,
      stroke: ctx.style.color,
      strokeWidth: ctx.style.strokeWidth,
    };
    return stroke;
  },
};
