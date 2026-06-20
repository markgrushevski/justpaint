// packages/editor/src/tools/eraser.ts — the eraser tool.
import type { FreehandPoint, FreehandStroke } from "@justpaint/document";
import type { LogicalPoint, Tool, ToolContext } from "../types";

/**
 * Eraser — a freehand stroke that erases earlier content on its own layer.
 *
 * Identical to the pen in every respect EXCEPT `composite`: the eraser is
 * `destination-out` (cuts pixels) where the pen is `source-over` (paints). The
 * renderer ignores `color` for `destination-out` strokes, but we still set it
 * from `ctx.style.color` so every freehand stroke has the same shape (§5.3).
 *
 * PURE: derives the stroke entirely from `ctx` + `gesture`. No Konva, no
 * module-level state, no rounding (serialization owns precision, §2).
 *
 * Like the pen, the eraser never produces a degenerate stroke that must be
 * discarded — a single sample is a valid 1-point dot (§5.3, "a dot is valid").
 * Only a truly empty gesture (zero samples) yields `null`.
 */
export const eraserTool: Tool = {
  id: "eraser",

  buildStroke(ctx: ToolContext, gesture: readonly LogicalPoint[]): FreehandStroke | null {
    if (gesture.length < 1) return null;

    const points: FreehandPoint[] = gesture.map((p) => [p.x, p.y, p.pressure]);

    return {
      id: ctx.newId(),
      type: "freehand",
      composite: "destination-out",
      color: ctx.style.color,
      points,
      brush: ctx.style.brush,
    };
  },
};
