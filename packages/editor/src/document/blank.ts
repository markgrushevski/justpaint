import { newId } from '../ids'
import { DEFAULT_CANVAS, DOC_VERSION } from './constants'
import type { Document } from './types'

/**
 * A new document with one empty layer. The background is null, so the canvas is
 * transparent and a host's view-only backdrop shows through it.
 */
export function blankDocument(width: number = DEFAULT_CANVAS.width, height: number = DEFAULT_CANVAS.height): Document {
    return {
        version: DOC_VERSION,
        width,
        height,
        background: null,
        layers: [{ id: newId(), name: 'Layer 1', visible: true, opacity: 1, strokes: [] }]
    }
}
