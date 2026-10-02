import { BRUSH_DEFAULTS } from '../src/document'
import { describe, expect, it } from 'vitest'
import { penTool } from '../src/tools/pen'
import type { LogicalPoint, ToolContext } from '../src/types'

/** A minimal ToolContext stub — fixed style + deterministic id. */
const ctx: ToolContext = {
    style: {
        color: '#1b1b1b',
        fill: '#cfe8ff',
        strokeWidth: 4,
        brush: BRUSH_DEFAULTS
    },
    newId: () => 's1'
}

describe('penTool', () => {
    it('id is pen', () => {
        expect(penTool.id).toBe('pen')
    })

    // Test A: a normal multi-point gesture builds the expected freehand stroke.
    it('builds a freehand source-over stroke from all gesture points', () => {
        const gesture: LogicalPoint[] = [
            { x: 10, y: 20, pressure: 0.4 },
            { x: 12.5, y: 24.1, pressure: 0.55 },
            { x: 30, y: 40, pressure: 0.61 }
        ]

        const stroke = penTool.buildStroke(ctx, gesture)
        expect(stroke).not.toBeNull()
        if (stroke === null) throw new Error('unreachable')

        expect(stroke.type).toBe('freehand')
        if (stroke.type !== 'freehand') throw new Error('unreachable')

        expect(stroke.id).toBe('s1')
        expect(stroke.composite).toBe('source-over')
        expect(stroke.color).toBe('#1b1b1b')
        // The default width (4) gives a brush of 12; every other option is the default.
        expect(stroke.brush).toEqual({ ...BRUSH_DEFAULTS, size: 12 })

        // points = gesture.map(p => [p.x, p.y, p.pressure]) — raw, unrounded.
        expect(stroke.points).toEqual([
            [10, 20, 0.4],
            [12.5, 24.1, 0.55],
            [30, 40, 0.61]
        ])
    })

    // Test B (pen variant): pen never returns null — a single-point gesture
    // yields a valid 1-point freehand stroke (a dot).
    it('treats a single-point gesture as a valid 1-point dot', () => {
        const gesture: LogicalPoint[] = [{ x: 50, y: 50, pressure: 0.5 }]

        const stroke = penTool.buildStroke(ctx, gesture)
        expect(stroke).not.toBeNull()
        if (stroke === null) throw new Error('unreachable')
        if (stroke.type !== 'freehand') throw new Error('unreachable')

        expect(stroke.points).toEqual([[50, 50, 0.5]])
        expect(stroke.points).toHaveLength(1)
    })

    // The freehand brush size follows the toolbar width (strokeWidth * 3); the rest of the brush is untouched.
    it.each([
        [1, 3],
        [4, 12],
        [10, 30],
        [64, 192]
    ])('strokeWidth %d builds a brush of size %d', (strokeWidth, size) => {
        const wide: ToolContext = { ...ctx, style: { ...ctx.style, strokeWidth } }

        const stroke = penTool.buildStroke(wide, [{ x: 1, y: 2, pressure: 0.5 }])
        if (stroke === null || stroke.type !== 'freehand') throw new Error('unreachable')

        expect(stroke.brush).toEqual({ ...BRUSH_DEFAULTS, size })
    })

    it('does not mutate the style it reads the brush from', () => {
        const wide: ToolContext = { ...ctx, style: { ...ctx.style, strokeWidth: 10 } }

        penTool.buildStroke(wide, [{ x: 1, y: 2, pressure: 0.5 }])

        expect(wide.style.brush).toBe(BRUSH_DEFAULTS)
        expect(BRUSH_DEFAULTS.size).toBe(16)
    })
})
