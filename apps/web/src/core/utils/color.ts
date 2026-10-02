/**
 * Colour math for an accent the player picks. The fill moves just far enough to clear 3:1
 * against the page, the bar the presets meet for the focus ring and the wordmark. Its label
 * ink is black or white, whichever reads better: on any fill one of them clears 4.5:1.
 */
import { colord, extend } from 'colord'
import a11yPlugin from 'colord/plugins/a11y'

extend([a11yPlugin])

/** The four primary sources `main.css` sets per preset. */
export interface AccentSources {
    primaryLight: string
    onPrimaryLight: string
    primaryDark: string
    onPrimaryDark: string
}

const PAGE_BAR = 3
const STEP = 0.02

/** Darken (on a light page) or lighten (on a dark one) until the fill clears the page bar. */
function againstPage(color: string, page: string): string {
    const towardsDark = colord(page).isLight()
    let c = colord(color)
    for (let i = 0; i < 50 && c.contrast(page) < PAGE_BAR; i++) {
        c = towardsDark ? c.darken(STEP) : c.lighten(STEP)
    }
    return c.toHex()
}

/** Pure black, not a near-black: a softer ink loses the 4.5:1 guarantee on mid-tone fills. */
export function inkOn(fill: string): '#000000' | '#ffffff' {
    return colord(fill).contrast('#000000') >= colord(fill).contrast('#ffffff') ? '#000000' : '#ffffff'
}

/** Sources for a picked colour, given the light and dark page backgrounds. */
export function accentSources(color: string, pageLight: string, pageDark: string): AccentSources {
    const primaryLight = againstPage(color, pageLight)
    const primaryDark = againstPage(color, pageDark)
    return {
        primaryLight,
        onPrimaryLight: inkOn(primaryLight),
        primaryDark,
        onPrimaryDark: inkOn(primaryDark)
    }
}

/** Whether ink on this colour should be light; null (no colour) reads as white paper. */
export function isDarkColor(color: string | null): boolean {
    return color !== null && colord(color).isDark()
}
