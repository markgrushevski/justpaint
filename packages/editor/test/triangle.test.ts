import { describe, expect, it } from 'vitest'
import { BRUSH_DEFAULTS } from '../src/document'
import { triangleTool } from '../src/tools/triangle'
import type { LogicalPoint, ToolContext } from '../src/types'

/** Minimal ctx stub: fixed style + deterministic id. */
const ctx: ToolContext = {
    style: {
        color: '#1b1b1b',
        fill: '#cfe8ff',
        strokeWidth: 4,
        brush: BRUSH_DEFAULTS
    },
    newId: () => 's1'
}

const pt = (x: number, y: number): LogicalPoint => ({ x, y, pressure: 0.5 })

describe('triangleTool', () => {
    it("id is 'triangle'", () => {
        expect(triangleTool.id).toBe('triangle')
    })

    // Test A: a normal gesture builds the expected apex-top triangle.
    it('builds the expected closed 3-point polygon', () => {
        // Drag bottom-right → top-left to also exercise bbox normalization.
        const gesture: readonly LogicalPoint[] = [pt(60, 80), pt(20, 30)]
        const stroke = triangleTool.buildStroke(ctx, gesture)

        expect(stroke).not.toBeNull()
        if (stroke === null) return // narrow for TS

        expect(stroke.type).toBe('polygon')
        expect(stroke.id).toBe('s1')
        expect(stroke.composite).toBe('source-over')
        expect(stroke.fill).toBe('#cfe8ff')
        expect(stroke.stroke).toBe('#1b1b1b')
        expect(stroke.strokeWidth).toBe(4)
        expect(stroke.join).toBe('round')

        // Normalized bbox: x=20, y=30, w=40, h=50; cx = 20 + 40/2 = 40.
        // apex-top: [cx,y], [x+w,y+h], [x,y+h].
        if (stroke.type !== 'polygon') return
        expect(stroke.points).toEqual([
            [40, 30],
            [60, 80],
            [20, 80]
        ])
    })

    // Test B: a degenerate gesture (zero height) returns null.
    it('returns null for a degenerate gesture (no area)', () => {
        // Same y for both samples → height 0.
        const flat: readonly LogicalPoint[] = [pt(10, 40), pt(70, 40)]
        expect(triangleTool.buildStroke(ctx, flat)).toBeNull()

        // Same x for both samples → width 0.
        const thin: readonly LogicalPoint[] = [pt(40, 10), pt(40, 70)]
        expect(triangleTool.buildStroke(ctx, thin)).toBeNull()
    })
})
