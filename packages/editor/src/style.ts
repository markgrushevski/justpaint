import { BRUSH_DEFAULTS } from './document'
import type { BrushOptions } from './document'
import type { Tool, ToolId, ToolStyle } from './types'

/**
 * The editor's default drawing style. `color`/`fill`/`strokeWidth` live here
 * (not in the document) so defaults are identical everywhere (DOCUMENT-FORMAT.md
 * §5.2).
 */
export const DEFAULT_STYLE: ToolStyle = {
    color: '#1b1b1b',
    fill: null,
    strokeWidth: 4,
    brush: BRUSH_DEFAULTS
}

/** Freehand brush `size` per unit of `strokeWidth`; the default width (4) gives a brush of 12. */
export const FREEHAND_SIZE_PER_WIDTH = 3

/** The brush a freehand tool stores on its stroke: `style.brush` with `size` taken from the width. */
export function freehandBrush(style: ToolStyle): BrushOptions {
    return { ...style.brush, size: style.strokeWidth * FREEHAND_SIZE_PER_WIDTH }
}

/** The tools whose strokes are freehand outlines sized by {@link freehandBrush}. */
const FREEHAND_TOOL_IDS: ReadonlySet<ToolId> = new Set<ToolId>(['pen', 'eraser'])

/**
 * The diameter, in logical units, that `tool` paints at under `style`: the brush size for a
 * freehand tool, `strokeWidth` for every other tool. The cursor ring shows this.
 */
export function toolDiameter(tool: Tool, style: ToolStyle): number {
    return FREEHAND_TOOL_IDS.has(tool.id) ? freehandBrush(style).size : style.strokeWidth
}
