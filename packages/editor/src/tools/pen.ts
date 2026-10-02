import type { FreehandStroke } from '../document'
import { freehandBrush } from '../style'
import type { LogicalPoint, StrokeTool, ToolContext } from '../types'

/**
 * Pen / brush — the freehand tool (DOCUMENT-FORMAT.md §5.3). Stores raw input
 * points (`[x, y, pressure]`), never the rendered outline — the renderer runs
 * `getStroke(points, brush)` later. Rounds nothing here; serialization handles
 * write-precision (§2). The brush size follows `strokeWidth` ({@link freehandBrush}).
 *
 * Never returns null, unlike other stroke tools: a single sample is already a
 * valid 1-point dot.
 */
export const penTool: StrokeTool = {
    kind: 'stroke',
    id: 'pen',
    buildStroke(ctx: ToolContext, gesture: readonly LogicalPoint[]): FreehandStroke | null {
        if (gesture.length < 1) return null

        return {
            id: ctx.newId(),
            type: 'freehand',
            composite: 'source-over',
            color: ctx.style.color,
            points: gesture.map((p) => [p.x, p.y, p.pressure]),
            brush: freehandBrush(ctx.style)
        }
    }
}
