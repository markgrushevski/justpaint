import { BRUSH_DEFAULTS } from './document'
import type { ToolStyle } from './types'

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
