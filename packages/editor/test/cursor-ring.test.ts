/**
 * The cursor ring's diameter is what the active tool will paint: the freehand brush
 * size for the pen and the eraser, `strokeWidth` for every other stroke tool.
 *
 * Headless like backdrop.test.ts: node-canvas backs Konva via "konva/canvas-backend"
 * (import before any stage exists), and the editor's DOM surfaces are stubbed.
 */
import 'konva/canvas-backend'
import Konva from 'konva'
import { afterEach, describe, expect, it } from 'vitest'
import type { Document } from '../src/document'
import { Editor } from '../src/editor'
import { TOOLS } from '../src/tools/index'
import type { StrokeToolId } from '../src/types'

// --- headless stubs (plain node env; installed once, before any Editor) -----

class StubResizeObserver {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
}

const g = globalThis as { ResizeObserver?: unknown; window?: unknown }
g.ResizeObserver ??= StubResizeObserver
g.window ??= { addEventListener: (): void => {}, removeEventListener: (): void => {} }

const VIEWPORT = { width: 800, height: 600 }

function fakeContainer(): HTMLDivElement {
    return {
        clientWidth: VIEWPORT.width,
        clientHeight: VIEWPORT.height,
        style: { cursor: '' },
        addEventListener: (): void => {},
        removeEventListener: (): void => {}
    } as unknown as HTMLDivElement
}

function doc(): Document {
    return {
        version: 1,
        width: 100,
        height: 100,
        background: '#ffffff',
        layers: [{ id: 'L1', name: 'L1', visible: true, opacity: 1, strokes: [] }]
    }
}

// --- editor plumbing ----------------------------------------------------------

let ed: Editor | null = null
afterEach(() => {
    ed?.destroy()
    ed = null
})

/** An editor with the ring on and the mouse hovering over the document. */
function hovering(): Editor {
    const e = new Editor(fakeContainer(), doc())
    ed = e
    e.setCursorColor('#ff0000')
    const stage = (e as unknown as { stage: Konva.Stage }).stage
    const native = { type: 'pointermove', pointerId: 1, pointerType: 'mouse', clientX: 400, clientY: 300 }
    stage.setPointersPositions(native)
    stage.fire('pointermove', { evt: native })
    return e
}

function ringOf(e: Editor): Konva.Circle {
    const ring = (e as unknown as { cursorRing: Konva.Circle | null }).cursorRing
    if (!ring) throw new Error('no cursor ring')
    return ring
}

// --- tests ----------------------------------------------------------------------

const SHAPE_TOOLS: StrokeToolId[] = ['line', 'rect', 'ellipse', 'triangle']

describe('cursor ring diameter', () => {
    it.each(['pen', 'eraser'] as const)('%s: the brush size, three times the width', (id) => {
        const e = hovering()
        e.setTool(TOOLS[id])

        e.setStyle({ strokeWidth: 4 })
        expect(ringOf(e).visible()).toBe(true)
        expect(ringOf(e).radius()).toBe(6) // brush 12 across

        e.setStyle({ strokeWidth: 10 })
        expect(ringOf(e).radius()).toBe(15) // brush 30 across
    })

    it.each(SHAPE_TOOLS)('%s: the stroke width itself', (id) => {
        const e = hovering()
        e.setTool(TOOLS[id])

        e.setStyle({ strokeWidth: 4 })
        expect(ringOf(e).radius()).toBe(2)

        e.setStyle({ strokeWidth: 10 })
        expect(ringOf(e).radius()).toBe(5)
    })

    it('follows the tool when it changes, without a pointer move', () => {
        const e = hovering()
        e.setStyle({ strokeWidth: 10 })

        e.setTool(TOOLS.pen)
        expect(ringOf(e).radius()).toBe(15)
        e.setTool(TOOLS.line)
        expect(ringOf(e).radius()).toBe(5)
        e.setTool(TOOLS.eraser)
        expect(ringOf(e).radius()).toBe(15)
        e.setTool(TOOLS.rect)
        expect(ringOf(e).radius()).toBe(5)
    })
})
