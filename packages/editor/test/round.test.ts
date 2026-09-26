import { describe, expect, it } from 'vitest'
import type { Document } from '../src/document'
import { roundDocument } from '../src/document'

const doc: Document = {
    version: 1,
    width: 1920,
    height: 1080,
    background: '#ffffff',
    layers: [
        {
            id: 'lyr',
            name: 'Layer 1',
            visible: true,
            opacity: 1,
            strokes: [
                {
                    id: 'pen',
                    type: 'freehand',
                    composite: 'source-over',
                    color: '#1b1b1b',
                    points: [
                        [420.123456, 300.987654, 0.426666],
                        [432.5, 305.1, 0.55]
                    ],
                    brush: {
                        size: 16,
                        thinning: 0.5,
                        smoothing: 0.5,
                        streamline: 0.5,
                        simulatePressure: true,
                        taperStart: 0,
                        taperEnd: 0
                    }
                },
                {
                    id: 'box',
                    type: 'rect',
                    composite: 'source-over',
                    x: 10.111,
                    y: 20.999,
                    width: 100.5,
                    height: 60.25,
                    fill: '#cfe8ff'
                }
            ]
        }
    ],
    meta: { generator: 'justpaint-test', freehandVersion: '1.2.4' }
}

describe('roundDocument', () => {
    it('rounds geometry to 2 dp and pressure to 3 dp', () => {
        const json = JSON.parse(JSON.stringify(roundDocument(doc))) as Document
        const pen = json.layers[0]!.strokes[0]!
        if (pen.type !== 'freehand') throw new Error('expected freehand')
        expect(pen.points[0]).toEqual([420.12, 300.99, 0.427])

        const box = json.layers[0]!.strokes[1]!
        if (box.type !== 'rect') throw new Error('expected rect')
        expect(box.x).toBe(10.11)
        expect(box.y).toBe(21)
    })

    it('drops absent optional channels from the payload', () => {
        const json = JSON.parse(JSON.stringify(roundDocument(doc))) as Document
        const box = json.layers[0]!.strokes[1]!
        expect('stroke' in box).toBe(false)
        expect('strokeWidth' in box).toBe(false)
    })

    it('never rounds a size the server requires to be positive down to 0', () => {
        const tiny: Document = {
            ...doc,
            layers: [
                {
                    ...doc.layers[0]!,
                    strokes: [
                        { id: 'r', type: 'rect', composite: 'source-over', x: 1, y: 1, width: 5, height: 0.004 },
                        { id: 'e', type: 'ellipse', composite: 'source-over', cx: 1, cy: 1, rx: 0.0045, ry: 3 },
                        {
                            id: 'l',
                            type: 'line',
                            composite: 'source-over',
                            points: [
                                [0, 0],
                                [5, 5]
                            ],
                            stroke: '#000000',
                            strokeWidth: 0.001
                        }
                    ]
                }
            ]
        }
        const [rect, ellipse, line] = roundDocument(tiny).layers[0]!.strokes
        expect(rect?.type === 'rect' && rect.height).toBe(0.01)
        expect(ellipse?.type === 'ellipse' && ellipse.rx).toBe(0.01)
        expect(line?.type === 'line' && line.strokeWidth).toBe(0.01)
    })

    it('leaves the input untouched', () => {
        roundDocument(doc)
        const pen = doc.layers[0]!.strokes[0]!
        if (pen.type !== 'freehand') throw new Error('expected freehand')
        expect(pen.points[0]).toEqual([420.123456, 300.987654, 0.426666])
    })
})
