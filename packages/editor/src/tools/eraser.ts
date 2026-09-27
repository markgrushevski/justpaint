import type { FreehandPoint, FreehandStroke } from '../document'
import type { LogicalPoint, StrokeTool, ToolContext } from '../types'

/**
 * Eraser — a freehand stroke that erases earlier content on its own layer.
 * Identical to the pen except `composite`: the eraser is `destination-out`
 * (cuts pixels) where the pen is `source-over` (paints). The renderer ignores
 * `color` for `destination-out` strokes, but it's still set from
 * `ctx.style.color` so every freehand stroke has the same shape (§5.3).
 *
 * Like the pen, only a truly empty gesture (zero samples) returns `null` — a
 * single sample is a valid 1-point dot (§5.3).
 */
export const eraserTool: StrokeTool = {
    kind: 'stroke',
    id: 'eraser',

    buildStroke(ctx: ToolContext, gesture: readonly LogicalPoint[]): FreehandStroke | null {
        if (gesture.length < 1) return null

        const points: FreehandPoint[] = gesture.map((p) => [p.x, p.y, p.pressure])

        return {
            id: ctx.newId(),
            type: 'freehand',
            composite: 'destination-out',
            color: ctx.style.color,
            points,
            brush: ctx.style.brush
        }
    }
}
