import { BRUSH_DEFAULTS, DOC_VERSION, roundDocument } from '../src/document'
import type { Document, Stroke } from '../src/document'
import { describe, expect, it } from 'vitest'
import { addLayerCommand, addStrokeCommand, compositeCommand, setLayerOpacityCommand } from '../src/history'
import { ellipseTool } from '../src/tools/ellipse'
import { eraserTool } from '../src/tools/eraser'
import { lineTool } from '../src/tools/line'
import { penTool } from '../src/tools/pen'
import { rectTool } from '../src/tools/rect'
import { triangleTool } from '../src/tools/triangle'
import type { LogicalPoint, StrokeTool, ToolStyle } from '../src/types'

/**
 * A document built with every stroke tool and the layer commands, rounded the way
 * the app sends it. The snapshot is the input of the server's TestEditorDocument,
 * so a tool or command that starts producing something the server rejects fails
 * there.
 */

// Deterministic ids shaped like newId()'s `stk_<uuid>`.
let seq = 0
const newId = (): string => `stk_00000000-0000-4000-8000-${String(++seq).padStart(12, '0')}`

const filled: ToolStyle = { color: '#1b1b1b', fill: '#cfe8ff', strokeWidth: 4, brush: BRUSH_DEFAULTS }
const outline: ToolStyle = { ...filled, fill: null, strokeWidth: 2 }

const drag: LogicalPoint[] = [
    { x: 100.123456, y: 120.987654, pressure: 0.4321 },
    { x: 180.5, y: 210.25, pressure: 0.55 },
    { x: 260.333333, y: 300.666666, pressure: 0.61 }
]
const backwards: LogicalPoint[] = [...drag].reverse()
const dot: LogicalPoint[] = [{ x: 540.5, y: 540.5, pressure: 0.5 }]
// Thinner than the write precision: a sub-pixel drag at high zoom.
const sliver: LogicalPoint[] = [
    { x: 10, y: 10, pressure: 0.5 },
    { x: 30, y: 10.004, pressure: 0.5 }
]

function draw(tool: StrokeTool, style: ToolStyle, gesture: LogicalPoint[]): Stroke {
    const stroke = tool.buildStroke({ style, newId }, gesture)
    if (stroke === null) throw new Error(`${tool.id} built nothing`)
    return stroke
}

const doc: Document = {
    version: DOC_VERSION,
    width: 1080,
    height: 1080,
    background: null,
    layers: [{ id: newId(), name: 'Layer 1', visible: true, opacity: 1, strokes: [] }]
}
const base = doc.layers[0]!.id
const top = newId()

const strokes = [
    ...[penTool, eraserTool].flatMap((t) => [draw(t, filled, drag), draw(t, filled, dot)]),
    ...[lineTool, rectTool, ellipseTool, triangleTool].flatMap((t) => [
        draw(t, filled, drag),
        draw(t, outline, backwards)
    ]),
    ...[rectTool, ellipseTool].map((t) => draw(t, outline, sliver))
]
compositeCommand(
    strokes.map((s) => addStrokeCommand(base, s)),
    'tools'
).apply(doc)
addLayerCommand({ id: top, name: 'Roof', visible: true, opacity: 1, strokes: [] }, 1).apply(doc)
setLayerOpacityCommand(doc, top, 0.5).apply(doc)
addStrokeCommand(top, draw(rectTool, filled, backwards)).apply(doc)

describe('server fixture', () => {
    it('matches the document the server test validates', async () => {
        await expect(JSON.stringify(roundDocument(doc), null, 2) + '\n').toMatchFileSnapshot(
            '../../../server/internal/document/testdata/editor-document.json'
        )
    })
})
