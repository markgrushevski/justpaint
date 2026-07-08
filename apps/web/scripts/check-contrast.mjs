#!/usr/bin/env node
/**
 * check-contrast.mjs — machine-checks the WCAG contrast claims made in the
 * comments of apps/web/src/main.css against the actual token values.
 *
 * Parses the brand-token custom properties (hex and `hsl(H S% L%)` forms) out
 * of main.css and uses **colord** (+ its a11y plugin) to compute WCAG 2.x
 * contrast ratios, then asserts the palette matrix:
 *
 *   TEXT     >= 4.5:1  (WCAG 1.4.3 AA)      — every on-* ink vs its ground,
 *                                             the dark danger override.
 *   NON-TEXT >= 3.0:1  (WCAG 1.4.11)        — outlines vs their grounds,
 *                                             the primary focus ring vs the page.
 *
 * Values are greped from main.css itself, so a palette retune is re-verified
 * automatically on every `npm run lint:all`. Exit 1 on any failing pair.
 */

import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { colord, extend } from 'colord'
import a11yPlugin from 'colord/plugins/a11y'

extend([a11yPlugin])

const CSS_PATH = join(dirname(fileURLToPath(import.meta.url)), '..', 'src', 'main.css')
const css = readFileSync(CSS_PATH, 'utf8')

/* ---------------------------------------------------------------- parsing */

/** First `{...}` body whose selector matches `selectorRe` (no nested braces in main.css). */
function block(selectorRe, label) {
    const re = new RegExp(selectorRe.source + '\\s*\\{([\\s\\S]*?)\\}', selectorRe.flags)
    const m = re.exec(css)
    if (!m) fail(`could not find the ${label} rule in ${CSS_PATH}`)
    return m[1]
}

/** Raw declaration value of `--name` inside a block. Anchored so `--ori-color:` never matches `--ori-color-*:`. */
function prop(body, name, label) {
    const re = new RegExp(`(?:^|[^\\w-])${name.replace(/[-]/g, '\\-')}\\s*:\\s*([^;]+);`, 'm')
    const m = re.exec(body)
    if (!m) fail(`token ${name} not found in the ${label} block of ${CSS_PATH}`)
    return m[1].trim()
}

/** #rgb | #rrggbb | hsl(H S% L%) -> a parsed colord() instance (colord parses all three natively). */
function parseColor(value, name) {
    const c = colord(value)
    if (!c.isValid()) {
        fail(`token ${name} has unsupported color value "${value}" (expected #rgb, #rrggbb or hsl(H S% L%))`)
    }
    return c
}

/* ------------------------------------------------------------------- WCAG */

function fail(msg) {
    console.error(`check-contrast: ${msg}`)
    process.exit(1)
}

/* ----------------------------------------------------------- token intake */

const root = block(/:root(?![.:])/, ':root brand-token')
const dark = block(/:root\.ori-theme_dark/, ':root.ori-theme_dark override')

const tokens = {}
for (const name of [
    'primary-light',
    'on-primary-light',
    'primary-dark',
    'on-primary-dark',
    'surface-light',
    'on-surface-light',
    'surface-dark',
    'on-surface-dark',
    'background-light',
    'on-background-light',
    'background-dark',
    'on-background-dark',
    'outline-light',
    'outline-dark'
]) {
    tokens[name] = parseColor(prop(root, `--ori-color-${name}`, ':root'), `--ori-color-${name}`)
}
// The letterbox desk tokens: parsed + validated (a rename/typo fails the run),
// but no contrast assertion — nothing is required to read against the desk.
for (const name of ['desk-light', 'desk-dark']) {
    tokens[name] = parseColor(prop(root, `--jp-${name}`, ':root'), `--jp-${name}`)
}
// Dark-only danger override (oriui's light-tuned red is too dim on our dark surfaces).
tokens['danger-dark'] = parseColor(prop(dark, '--ori-color-danger', 'dark'), '--ori-color-danger (dark)')
// Role-as-text AA (outline/tonal/text buttons, selected tab, tag, link) is now owned by oriui's
// --ori-color-<role>-text tokens (a derived color-mix, guarded by oriui's e2e/text-contrast.spec.ts),
// so this app no longer pins or re-verifies a primary text tone here.

/* ----------------------------------------------------------------- matrix */

const TEXT = 4.5 // WCAG 1.4.3 AA, normal text
const NON_TEXT = 3.0 // WCAG 1.4.11, UI components / graphical objects

/** [foreground, background, minimum] */
const MATRIX = [
    // TEXT >= 4.5
    ['on-surface-light', 'surface-light', TEXT],
    ['on-surface-dark', 'surface-dark', TEXT],
    ['on-background-light', 'background-light', TEXT],
    ['on-background-dark', 'background-dark', TEXT],
    ['on-primary-light', 'primary-light', TEXT],
    ['on-primary-dark', 'primary-dark', TEXT],
    ['danger-dark', 'surface-dark', TEXT],
    // NON-TEXT >= 3.0
    ['outline-light', 'surface-light', NON_TEXT],
    ['outline-light', 'background-light', NON_TEXT],
    ['outline-dark', 'surface-dark', NON_TEXT],
    ['outline-dark', 'background-dark', NON_TEXT],
    ['primary-light', 'background-light', NON_TEXT], // focus ring on the page
    ['primary-dark', 'background-dark', NON_TEXT] // focus ring on the page
]

const failures = []
const rows = []
for (const [fg, bg, min] of MATRIX) {
    const ratio = tokens[fg].contrast(tokens[bg])
    const pair = `${fg} vs ${bg}`
    const kind = min === TEXT ? 'text' : 'non-text'
    rows.push({ pair, kind, min, ratio })
    if (ratio < min) failures.push({ pair, kind, min, ratio })
}

const pairWidth = Math.max(...rows.map((r) => r.pair.length))
for (const { pair, kind, min, ratio } of rows) {
    const status = ratio >= min ? 'PASS' : 'FAIL'
    console.log(
        `${status}  ${pair.padEnd(pairWidth)}  ${ratio.toFixed(2).padStart(5)}:1  (${kind} >= ${min.toFixed(1)}:1)`
    )
}
console.log(`      desk tokens parsed ok (no contrast requirement): --jp-desk-light, --jp-desk-dark`)

if (failures.length > 0) {
    console.error(`\ncheck-contrast: ${failures.length} pair(s) below the WCAG bar:`)
    for (const { pair, kind, min, ratio } of failures) {
        console.error(`  ${pair}: ${ratio.toFixed(2)}:1 — required ${kind} minimum is ${min.toFixed(1)}:1`)
    }
    process.exit(1)
}
console.log(`\ncheck-contrast: all ${rows.length} pairs pass.`)
