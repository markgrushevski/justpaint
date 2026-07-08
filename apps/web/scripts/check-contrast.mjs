#!/usr/bin/env node
/**
 * check-contrast.mjs — machine-checks the WCAG contrast claims made in the
 * comments of apps/web/src/main.css against the actual token values.
 *
 * Zero dependencies. Parses the brand-token custom properties (hex and
 * `hsl(H S% L%)` forms), computes WCAG 2.x relative luminance + contrast
 * ratios, and asserts the palette matrix:
 *
 *   TEXT     >= 4.5:1  (WCAG 1.4.3 AA)      — every on-* ink vs its ground,
 *                                             the light primary TEXT tone,
 *                                             the dark danger override.
 *   NON-TEXT >= 3.0:1  (WCAG 1.4.11)        — outlines vs their grounds,
 *                                             the primary focus ring vs the page.
 *
 * Values are greped from main.css itself, so a palette retune is re-verified
 * automatically on every `npm run lint:all`. Exit 1 on any failing pair.
 */

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const CSS_PATH = join(dirname(fileURLToPath(import.meta.url)), '..', 'src', 'main.css');
const css = readFileSync(CSS_PATH, 'utf8');

/* ---------------------------------------------------------------- parsing */

/** First `{...}` body whose selector matches `selectorRe` (no nested braces in main.css). */
function block(selectorRe, label) {
    const re = new RegExp(selectorRe.source + '\\s*\\{([\\s\\S]*?)\\}', selectorRe.flags);
    const m = re.exec(css);
    if (!m) fail(`could not find the ${label} rule in ${CSS_PATH}`);
    return m[1];
}

/** Raw declaration value of `--name` inside a block. Anchored so `--ori-color:` never matches `--ori-color-*:`. */
function prop(body, name, label) {
    const re = new RegExp(`(?:^|[^\\w-])${name.replace(/[-]/g, '\\-')}\\s*:\\s*([^;]+);`, 'm');
    const m = re.exec(body);
    if (!m) fail(`token ${name} not found in the ${label} block of ${CSS_PATH}`);
    return m[1].trim();
}

/** #rgb | #rrggbb | hsl(H S% L%) -> [r, g, b] in 0..255. */
function parseColor(value, name) {
    let m = /^#([0-9a-f]{3})$/i.exec(value);
    if (m) return [...m[1]].map((c) => parseInt(c + c, 16));
    m = /^#([0-9a-f]{6})$/i.exec(value);
    if (m) return [0, 2, 4].map((i) => parseInt(m[1].slice(i, i + 2), 16));
    m = /^hsl\(\s*(-?[\d.]+)(?:deg)?\s+([\d.]+)%\s+([\d.]+)%\s*\)$/i.exec(value);
    if (m) return hslToRgb(Number(m[1]), Number(m[2]) / 100, Number(m[3]) / 100);
    fail(`token ${name} has unsupported color value "${value}" (expected #rgb, #rrggbb or hsl(H S% L%))`);
}

function hslToRgb(h, s, l) {
    h = ((h % 360) + 360) % 360;
    const c = (1 - Math.abs(2 * l - 1)) * s;
    const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
    const m = l - c / 2;
    const sextant = Math.floor(h / 60) % 6;
    const rgb1 = [
        [c, x, 0],
        [x, c, 0],
        [0, c, x],
        [0, x, c],
        [x, 0, c],
        [c, 0, x],
    ][sextant];
    return rgb1.map((v) => Math.round((v + m) * 255));
}

/* ------------------------------------------------------------------- WCAG */

function luminance([r, g, b]) {
    const lin = (c8) => {
        const c = c8 / 255;
        return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    };
    return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

function contrast(a, b) {
    const [hi, lo] = luminance(a) > luminance(b) ? [a, b] : [b, a];
    return (luminance(hi) + 0.05) / (luminance(lo) + 0.05);
}

function fail(msg) {
    console.error(`check-contrast: ${msg}`);
    process.exit(1);
}

/* ----------------------------------------------------------- token intake */

const root = block(/:root(?![.:])/, ':root brand-token');
const dark = block(/:root\.ori-theme_dark/, ':root.ori-theme_dark override');
const lightButton = block(
    /:root:not\(\.ori-theme_dark\)[\s\S]*?\.ori-button[^{]*/,
    'light primary-button text-tone override',
);

const tokens = {};
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
    'outline-dark',
]) {
    tokens[name] = parseColor(prop(root, `--ori-color-${name}`, ':root'), `--ori-color-${name}`);
}
// The letterbox desk tokens: parsed + validated (a rename/typo fails the run),
// but no contrast assertion — nothing is required to read against the desk.
for (const name of ['desk-light', 'desk-dark']) {
    tokens[name] = parseColor(prop(root, `--jp-${name}`, ':root'), `--jp-${name}`);
}
// Dark-only danger override (oriui's light-tuned red is too dim on our dark surfaces).
tokens['danger-dark'] = parseColor(prop(dark, '--ori-color-danger', 'dark'), '--ori-color-danger (dark)');
// Light-theme darkened TEXT tone for primary outline/tonal/text buttons.
tokens['primary-text-light'] = parseColor(
    prop(lightButton, '--ori-color', 'light button override'),
    '--ori-color (light primary text tone)',
);

/* ----------------------------------------------------------------- matrix */

const TEXT = 4.5; // WCAG 1.4.3 AA, normal text
const NON_TEXT = 3.0; // WCAG 1.4.11, UI components / graphical objects

/** [foreground, background, minimum] */
const MATRIX = [
    // TEXT >= 4.5
    ['on-surface-light', 'surface-light', TEXT],
    ['on-surface-dark', 'surface-dark', TEXT],
    ['on-background-light', 'background-light', TEXT],
    ['on-background-dark', 'background-dark', TEXT],
    ['on-primary-light', 'primary-light', TEXT],
    ['on-primary-dark', 'primary-dark', TEXT],
    ['primary-text-light', 'surface-light', TEXT],
    ['primary-text-light', 'background-light', TEXT],
    ['danger-dark', 'surface-dark', TEXT],
    // NON-TEXT >= 3.0
    ['outline-light', 'surface-light', NON_TEXT],
    ['outline-light', 'background-light', NON_TEXT],
    ['outline-dark', 'surface-dark', NON_TEXT],
    ['outline-dark', 'background-dark', NON_TEXT],
    ['primary-light', 'background-light', NON_TEXT], // focus ring on the page
    ['primary-dark', 'background-dark', NON_TEXT], // focus ring on the page
];

const failures = [];
const rows = [];
for (const [fg, bg, min] of MATRIX) {
    const ratio = contrast(tokens[fg], tokens[bg]);
    const pair = `${fg} vs ${bg}`;
    const kind = min === TEXT ? 'text' : 'non-text';
    rows.push({ pair, kind, min, ratio });
    if (ratio < min) failures.push({ pair, kind, min, ratio });
}

const pairWidth = Math.max(...rows.map((r) => r.pair.length));
for (const { pair, kind, min, ratio } of rows) {
    const status = ratio >= min ? 'PASS' : 'FAIL';
    console.log(
        `${status}  ${pair.padEnd(pairWidth)}  ${ratio.toFixed(2).padStart(5)}:1  (${kind} >= ${min.toFixed(1)}:1)`,
    );
}
console.log(`      desk tokens parsed ok (no contrast requirement): --jp-desk-light, --jp-desk-dark`);

if (failures.length > 0) {
    console.error(`\ncheck-contrast: ${failures.length} pair(s) below the WCAG bar:`);
    for (const { pair, kind, min, ratio } of failures) {
        console.error(`  ${pair}: ${ratio.toFixed(2)}:1 — required ${kind} minimum is ${min.toFixed(1)}:1`);
    }
    process.exit(1);
}
console.log(`\ncheck-contrast: all ${rows.length} pairs pass.`);
