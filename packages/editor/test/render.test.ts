/**
 * renderToStage re-homes every document layer onto the output stage. It runs
 * headless here (node-canvas via konva/canvas-backend), like the render worker.
 */
import 'konva/canvas-backend'
import { describe, expect, it } from 'vitest'
import type { Document, Layer } from '../src/document'
import { renderToStage } from '../src/render'

function layer(id: string, x: number): Layer {
    return {
        id,
        name: id,
        visible: true,
        opacity: 1,
        strokes: [
            { id: `r-${id}`, type: 'rect', composite: 'source-over', x, y: 10, width: 20, height: 20, fill: '#000000' }
        ]
    }
}

function doc(...layers: Layer[]): Document {
    return { version: 1, width: 100, height: 100, background: null, layers }
}

/** Dark pixels in the rendered frame. */
function ink(d: Document): number {
    const stage = renderToStage(d, { outWidth: 100, outHeight: 100, background: '#ffffff' })
    try {
        const canvas = stage.toCanvas({ pixelRatio: 1 })
        const data = canvas.getContext('2d')!.getImageData(0, 0, 100, 100).data
        let n = 0
        for (let i = 0; i < data.length; i += 4) if (data[i]! < 128) n++
        return n
    } finally {
        stage.destroy()
    }
}

describe('renderToStage', () => {
    it('keeps every layer, not every other one', () => {
        const stage = renderToStage(doc(layer('a', 0), layer('b', 30), layer('c', 60)), {
            outWidth: 100,
            outHeight: 100,
            background: '#ffffff'
        })
        try {
            // The background layer plus the three content layers.
            expect(stage.getLayers()).toHaveLength(4)
        } finally {
            stage.destroy()
        }
    })

    it('draws the strokes of the second and third layers', () => {
        const one = ink(doc(layer('a', 0)))
        expect(one).toBeGreaterThan(0)
        expect(ink(doc(layer('a', 0), layer('b', 30), layer('c', 60)))).toBe(3 * one)
    })
})
