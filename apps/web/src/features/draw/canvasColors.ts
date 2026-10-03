/**
 * The canvas colours /draw offers. A drawing keeps the one it has (`doc.background`); null
 * is plain paper, white on screen and transparent in an export.
 */
import { DEFAULT_STYLE } from '@justpaint/editor'
import type { SwatchOption } from '../../components/ui/SwatchPicker.vue'

/** A new drawing's colour in the dark theme, unless the canvas is inverted. */
export const DARK_CANVAS = '#1c1b1a'

const INK_ON_LIGHT = '#242322'
const INK_ON_DARK = '#ffffff'

/** Light papers on the first row, dark ones on the second. */
export const CANVAS_COLORS: SwatchOption[] = [
    { value: null, label: 'Paper', color: '#ffffff', ink: INK_ON_LIGHT },
    { value: '#f2f0ec', label: 'Stone', color: '#f2f0ec', ink: INK_ON_LIGHT },
    { value: '#e4eefc', label: 'Sky', color: '#e4eefc', ink: INK_ON_LIGHT },
    { value: '#fcf2c8', label: 'Butter', color: '#fcf2c8', ink: INK_ON_LIGHT },
    { value: '#fbe2e8', label: 'Blush', color: '#fbe2e8', ink: INK_ON_LIGHT },
    { value: DARK_CANVAS, label: 'Charcoal', color: DARK_CANVAS, ink: INK_ON_DARK },
    { value: '#18202e', label: 'Night', color: '#18202e', ink: INK_ON_DARK },
    { value: '#17241d', label: 'Forest', color: '#17241d', ink: INK_ON_DARK },
    { value: '#261c2a', label: 'Plum', color: '#261c2a', ink: INK_ON_DARK }
]

/**
 * What a new drawing starts on: the theme's paper. An inverted canvas shows plain paper dark
 * already, so it starts on paper in both themes.
 */
export function defaultCanvas(dark: boolean, inverted: boolean): string | null {
    return dark && !inverted ? DARK_CANVAS : null
}

/** The pen's default colour on a light or a dark canvas. */
export const DEFAULT_INK = { light: DEFAULT_STYLE.color, dark: '#f1f0ee' } as const
