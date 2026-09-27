/**
 * Guards the hand-maintained oriUI stylesheet list in `src/main.ts` — a
 * missing import renders a component unstyled with no error (docs/NOTES.md,
 * "The oriui CSS import list is hand-maintained"). Checks by SELECTOR, not
 * filename, since some component CSS ships inside another file's stylesheet.
 */
import { readFileSync, readdirSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createRequire } from 'node:module'

const webRoot = join(dirname(fileURLToPath(import.meta.url)), '..')
const srcRoot = join(webRoot, 'src')
// Resolve via Node, not a guessed path — npm hoisting varies (docs/NOTES.md,
// "Tool scripts must resolve packages, not assume the root node_modules").
const require = createRequire(import.meta.url)
const cssComponents = join(dirname(require.resolve('@oriui/css/package.json')), 'dist', 'components')

/** Every file under src/, recursively. */
function walk(dir) {
    return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
        const full = join(dir, entry.name)
        return entry.isDirectory() ? walk(full) : [full]
    })
}

/**
 * Components whose block name doesn't follow from the component name: the
 * toolbar controls compose OriButton, the separator renders a toolbar BEM
 * element. Without these the guard would silently abstain instead of flagging them.
 */
const BLOCK_ALIASES = {
    OriToolbarButton: 'ori-button',
    OriToolbarToggleItem: 'ori-button',
    OriToolbarSeparator: 'ori-toolbar__separator'
}

/** `OriToolbarButton` -> `ori-toolbar-button` */
function toClass(component) {
    return component
        .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
        .replace(/([A-Z])([A-Z][a-z])/g, '$1-$2')
        .toLowerCase()
}

const sources = walk(srcRoot).filter((f) => /\.(vue|ts)$/.test(f))

// Actually-rendered components — an import alone would over-report (a
// type-only import, or a name mentioned in a comment), so require a tag.
const used = new Set()
for (const file of sources) {
    const text = readFileSync(file, 'utf8')
    for (const [, name] of text.matchAll(/<(Ori[A-Z][A-Za-z]*)[\s/>]/g)) used.add(name)
}

// The stylesheets main.ts pulls in, concatenated — this is what the browser gets.
const mainTs = readFileSync(join(srcRoot, 'main.ts'), 'utf8')
const imported = [...mainTs.matchAll(/@oriui\/css\/components\/([a-z-]+)\.css/g)].map((m) => m[1])
const loadedCss = imported.map((name) => readFileSync(join(cssComponents, `${name}.css`), 'utf8')).join('\n')

// Every class oriUI defines anywhere, to tell "unstyled" apart from "has no styles of its own".
const allCss = readdirSync(cssComponents)
    .filter((f) => f.endsWith('.css'))
    .map((f) => readFileSync(join(cssComponents, f), 'utf8'))
    .join('\n')

/**
 * Whole-class match, not substring: minified CSS turns up `.ori-badge,`,
 * `.ori-badge{`, `.ori-badge:hover` etc., and the lookahead stops `.ori-badge`
 * from also matching inside the unrelated `.ori-badge-anchor`.
 */
function hasClass(css, cls) {
    return new RegExp(`\.${cls}(?![\w-])`).test(css)
}

const missing = []
for (const component of [...used].sort()) {
    const cls = BLOCK_ALIASES[component] ?? toClass(component)
    // No block styles anywhere in oriUI to import; add a BLOCK_ALIASES entry if this ever abstains wrongly.
    if (!hasClass(allCss, cls)) continue
    if (!hasClass(loadedCss, cls)) missing.push({ component, cls })
}

if (missing.length > 0) {
    console.error('check-styles: components rendered without their oriUI stylesheet:\n')
    for (const { component, cls } of missing) {
        console.error(`  ${component} (.${cls}) — add the matching @oriui/css/components/*.css import to src/main.ts`)
    }
    console.error(`\n${missing.length} component(s) would render unstyled.`)
    process.exit(1)
}

console.log(
    `check-styles: ${used.size} Ori* components rendered, all styled by the ${imported.length} imported stylesheets.`
)
