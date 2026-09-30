/** "2 minutes ago", "yesterday": a timestamp relative to now, in English like the rest of the UI. */

const UNITS: readonly (readonly [Intl.RelativeTimeFormatUnit, number])[] = [
    ['year', 31_536_000],
    ['month', 2_592_000],
    ['week', 604_800],
    ['day', 86_400],
    ['hour', 3_600],
    ['minute', 60]
]

const formatter = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })

/** Empty for an unparseable timestamp; under a minute (or a clock a little ahead) reads "now". */
export function timeAgo(iso: string, now: number = Date.now()): string {
    const at = Date.parse(iso)
    if (Number.isNaN(at)) return ''
    const seconds = Math.round((at - now) / 1000)
    for (const [unit, size] of UNITS) {
        if (Math.abs(seconds) >= size) return formatter.format(Math.trunc(seconds / size), unit)
    }
    return formatter.format(0, 'second')
}
