import { describe, expect, it } from 'vitest'
import { BRUSH_DEFAULTS } from '../src/document'
import type { RectStroke } from '../src/document'
import type { ToolContext } from '../src/types'
// Import the tool DIRECTLY (not the barrel) so the test never pulls in Konva.
import { rectTool } from '../src/tools/rect'

const ctx: ToolContext = {
    style: {
        color: '#1b1b1b',
        fill: '#cfe8ff',
        strokeWidth: 4,
        brush: BRUSH_DEFAULTS
    },
    newId: () => 's1'
}

describe('rectTool', () => {
    it('builds a normalized rect from a negative drag', () => {
        // Drag from bottom-right (30,40) to top-left (10,15): must normalize.
        const stroke = rectTool.buildStroke(ctx, [
            { x: 30, y: 40, pressure: 0.5 },
            { x: 10, y: 15, pressure: 0.5 }
        ])

        expect(stroke).not.toBeNull()
        const rect = stroke as RectStroke

        expect(rect.type).toBe('rect')
        expect(rect.composite).toBe('source-over')
        expect(rect.id).toBe('s1')
        // Normalized top-left + non-negative extents.
        expect(rect.x).toBe(10)
        expect(rect.y).toBe(15)
        expect(rect.width).toBe(20)
        expect(rect.height).toBe(25)
        // Style passthrough.
        expect(rect.fill).toBe('#cfe8ff')
        expect(rect.stroke).toBe('#1b1b1b')
        expect(rect.strokeWidth).toBe(4)
    })

    it('returns null for a degenerate (zero-area) gesture', () => {
        // Last point shares the x of the first ⇒ zero width.
        const stroke = rectTool.buildStroke(ctx, [
            { x: 20, y: 10, pressure: 0.5 },
            { x: 20, y: 60, pressure: 0.5 }
        ])
        expect(stroke).toBeNull()
    })
})
