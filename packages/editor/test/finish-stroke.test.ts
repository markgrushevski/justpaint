/**
 * Editor.finishStroke — a host commits the stroke in progress before reading the
 * document (the duel's timed submit). Headless as in hand.test.ts.
 */
import 'konva/canvas-backend'
import Konva from 'konva'
import { afterEach, describe, expect, it } from 'vitest'
import { Editor } from '../src/editor'
import { TOOLS } from '../src/tools/index'
import type { Document } from '../src/document'

class StubResizeObserver {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
}

const g = globalThis as { ResizeObserver?: unknown; window?: unknown }
g.ResizeObserver ??= StubResizeObserver
g.window ??= { addEventListener: (): void => {}, removeEventListener: (): void => {} }

function fakeContainer(): HTMLDivElement {
    return {
        clientWidth: 800,
        clientHeight: 600,
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

// The 100×100 doc fit into 800×600: zoom 6, panX 100, panY 0.
function fire(stage: Konva.Stage, name: 'pointerdown' | 'pointermove' | 'pointerup', x: number, y: number): void {
    const native = {
        type: name,
        button: 0,
        pointerId: 1,
        pointerType: 'mouse',
        pressure: 0.5,
        clientX: x * 6 + 100,
        clientY: y * 6,
        preventDefault: (): void => {}
    }
    stage.setPointersPositions(native)
    stage.fire(name, { evt: native })
}

let ed: Editor | null = null
afterEach(() => {
    ed?.destroy()
    ed = null
})

function strokes(e: Editor): number {
    return e.getDocument().layers[0]?.strokes.length ?? 0
}

describe('Editor.finishStroke', () => {
    it('commits the stroke in progress, and the rest of that gesture draws nothing', () => {
        ed = new Editor(fakeContainer(), doc())
        ed.setTool(TOOLS.pen)
        const stage = (ed as unknown as { stage: Konva.Stage }).stage

        fire(stage, 'pointerdown', 10, 10)
        fire(stage, 'pointermove', 20, 20)
        expect(strokes(ed)).toBe(0)

        ed.finishStroke()
        expect(strokes(ed)).toBe(1)
        expect(ed.canUndo()).toBe(true)

        fire(stage, 'pointermove', 30, 30)
        fire(stage, 'pointerup', 40, 40)
        expect(strokes(ed)).toBe(1)
    })

    it('does nothing without a stroke in progress', () => {
        ed = new Editor(fakeContainer(), doc())
        ed.finishStroke()
        expect(strokes(ed)).toBe(0)
        expect(ed.canUndo()).toBe(false)
    })
})
