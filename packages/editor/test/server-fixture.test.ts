import { BRUSH_DEFAULTS, DOC_VERSION, roundDocument } from '../src/document'
import type { Document, Stroke } from '../src/document'
import { describe, expect, it } from 'vitest'
import { ellipseTool } from '../src/tools/ellipse'
import { eraserTool } from '../src/tools/eraser'
import { lineTool } from '../src/tools/line'
import { penTool } from '../src/tools/pen'
import { rectTool } from '../src/tools/rect'
import { triangleTool } from '../src/tools/triangle'
import type { LogicalPoint, StrokeTool, ToolContext } from '../src/types'

/**
 * A document drawn with every stroke tool and rounded the way the app sends it.
 * The snapshot is the input of the server's TestEditorDocument, so an editor
 * change the server would reject fails there.
 */

let seq = 0
const ctx: ToolContext = {
    style: { color: '#1b1b1b', fill: '#cfe8ff', strokeWidth: 4, brush: BRUSH_DEFAULTS },
    newId: () => `stk_${++seq}`
}

const gesture: LogicalPoint[] = [
    { x: 100.123456, y: 120.987654, pressure: 0.4321 },
    { x: 180.5, y: 210.25, pressure: 0.55 },
    { x: 260.333333, y: 300.666666, pressure: 0.61 }
]

function draw(tool: StrokeTool): Stroke {
    const stroke = tool.buildStroke(ctx, gesture)
    if (stroke === null) throw new Error(`${tool.id} built nothing`)
    return stroke
}

const doc: Document = {
    version: DOC_VERSION,
    width: 1080,
    height: 1080,
    background: null,
    layers: [
        {
            id: 'layer_1',
            name: 'Layer 1',
            visible: true,
            opacity: 1,
            strokes: [penTool, eraserTool, lineTool, rectTool, ellipseTool, triangleTool].map(draw)
        }
    ]
}

describe('server fixture', () => {
    it('matches the document the server test validates', async () => {
        await expect(JSON.stringify(roundDocument(doc), null, 2) + '\n').toMatchFileSnapshot(
            '../../../server/internal/document/testdata/editor-document.json'
        )
    })
})
