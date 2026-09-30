import { describe, expect, it } from 'vitest'
import { blankDocument, DEFAULT_CANVAS, DOC_VERSION } from '../src/document'

describe('blankDocument', () => {
    it('is one empty, visible layer on a transparent canvas', () => {
        const doc = blankDocument(1080, 1080)
        expect(doc).toMatchObject({ version: DOC_VERSION, width: 1080, height: 1080, background: null })
        expect(doc.layers).toHaveLength(1)
        expect(doc.layers[0]).toMatchObject({ name: 'Layer 1', visible: true, opacity: 1, strokes: [] })
    })

    it('defaults to the free-draw canvas and never reuses a layer id', () => {
        const a = blankDocument()
        const b = blankDocument()
        expect([a.width, a.height]).toEqual([DEFAULT_CANVAS.width, DEFAULT_CANVAS.height])
        expect(a.layers[0]?.id).not.toBe(b.layers[0]?.id)
    })
})
