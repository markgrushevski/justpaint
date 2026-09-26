import { describe, expect, it } from 'vitest'
import { BRUSH_DEFAULTS } from '@justpaint/document'

// Import the tool DIRECTLY (not via the barrel) so the test never pulls Konva in.
import { lineTool } from '../src/tools/line'
import type { LogicalPoint, ToolContext } from '../src/types'

/** A minimal pointer sample in logical coords. */
function pt(x: number, y: number, pressure = 0.5): LogicalPoint {
    return { x, y, pressure }
}

/** A deterministic ToolContext stub for the line tool. */
function makeCtx(): ToolContext {
    return {
        style: {
            color: '#1b1b1b',
            fill: '#cfe8ff',
            strokeWidth: 4,
            brush: BRUSH_DEFAULTS
        },
        newId: () => 's1'
    }
}

describe('lineTool', () => {
    it('A: builds a 2-point line from a normal gesture', () => {
        const ctx = makeCtx()
        const gesture: LogicalPoint[] = [pt(10, 20), pt(40, 25), pt(80, 60)]

        const stroke = lineTool.buildStroke(ctx, gesture)
        expect(stroke).not.toBeNull()
        // Narrow for type-safe field access below.
        if (stroke === null) throw new Error('expected a stroke')

        expect(stroke.type).toBe('line')
        expect(stroke.id).toBe('s1')
        expect(stroke.composite).toBe('source-over')
        // Endpoints come from gesture[0] and gesture[last]; the middle is ignored.
        expect(stroke.points).toEqual([
            [10, 20],
            [80, 60]
        ])
        expect(stroke.stroke).toBe('#1b1b1b')
        expect(stroke.strokeWidth).toBe(4)
        expect(stroke.cap).toBe('round')
        expect(stroke.join).toBe('round')
    })

    it('B: returns null for a zero-length (degenerate) gesture', () => {
        const ctx = makeCtx()
        // Start and end coincide → zero-length line, discarded.
        const gesture: LogicalPoint[] = [pt(50, 50), pt(50, 50, 0.9)]

        expect(lineTool.buildStroke(ctx, gesture)).toBeNull()
    })
})
