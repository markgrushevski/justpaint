import type { LineStroke } from "@justpaint/document";
import type { LogicalPoint, Tool, ToolContext } from "../types";

/**
 * Line tool — a straight, two-point segment (docs/DOCUMENT-FORMAT.md §5.4).
 *
 * Pure: the stroke is derived entirely from `ctx` + `gesture`, with no Konva,
 * no module-level state, and no side effects. The endpoints are the first and
 * last samples of the gesture (pointerdown → pointerup); everything in between
 * is ignored — a line is anchored at its ends, not its path.
 */
export const lineTool = {
  id: "line",
  buildStroke(ctx: ToolContext, gesture: readonly LogicalPoint[]): LineStroke | null {
    const start = gesture[0];
    const end = gesture[gesture.length - 1];
    if (start === undefined || end === undefined) return null;

    // Degenerate: zero-length line (both endpoints coincide) is discarded.
    if (start.x === end.x && start.y === end.y) return null;

    return {
      id: ctx.newId(),
      type: "line",
      composite: "source-over",
      points: [
        [start.x, start.y],
        [end.x, end.y],
      ],
      stroke: ctx.style.color,
      strokeWidth: ctx.style.strokeWidth,
      cap: "round",
      join: "round",
    };
  },
} satisfies Tool;
