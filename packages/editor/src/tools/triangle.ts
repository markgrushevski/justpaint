import type { PolygonStroke } from "@justpaint/document";
import type { LogicalPoint, Tool, ToolContext } from "../types";

/**
 * Triangle tool — builds a real closed 3-vertex {@link PolygonStroke} from the
 * drag bounding box (apex-top convention). Fixes the old engine's bug where the
 * "Triangle" tool drew a rectangle (`CanvasTool.ts:386-396`).
 *
 * PURE: derives everything from `ctx` + `gesture`. No Konva, no module state,
 * no side effects. The first gesture point is pointerdown, the last is the
 * current/pointerup; the bbox spans the gesture's min/max in x and y.
 */
export const triangleTool = {
  id: "triangle",

  buildStroke(
    ctx: ToolContext,
    gesture: readonly LogicalPoint[],
  ): PolygonStroke | null {
    const first = gesture[0];
    if (first === undefined) return null;

    // Normalized bbox over every sample — independent of drag direction.
    let minX = first.x;
    let minY = first.y;
    let maxX = first.x;
    let maxY = first.y;
    for (const p of gesture) {
      if (p.x < minX) minX = p.x;
      if (p.x > maxX) maxX = p.x;
      if (p.y < minY) minY = p.y;
      if (p.y > maxY) maxY = p.y;
    }

    const x = minX;
    const y = minY;
    const w = maxX - minX;
    const h = maxY - minY;

    // Degenerate drag: no area — discard. Editor-side nicety; the validator
    // itself accepts any polygon with >=3 finite vertices (no zero-area rule).
    if (w <= 0 || h <= 0) return null;

    const cx = x + w / 2;

    return {
      id: ctx.newId(),
      type: "polygon",
      composite: "source-over",
      points: [
        [cx, y], // apex (top-center)
        [x + w, y + h], // bottom-right
        [x, y + h], // bottom-left
      ],
      fill: ctx.style.fill,
      stroke: ctx.style.color,
      strokeWidth: ctx.style.strokeWidth,
      join: "round",
    };
  },
} satisfies Tool;
