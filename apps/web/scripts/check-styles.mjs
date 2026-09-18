/**
 * Guard for the hand-maintained oriUI stylesheet list in `src/main.ts`.
 *
 * We import oriUI's CSS à la carte — one file per component actually used — which
 * keeps the bundle honest but leaves a list that a human must remember to update.
 * It was already wrong: `OriBadge` and `OriSkeleton` were rendered on /leaderboard
 * and in the judging overlay with no block styles at all, because nobody added
 * their two lines (JP-I-01).
 *
 * The check is deliberately about SELECTORS, not filenames. Some component CSS is
 * inlined into another file — `.ori-spinner` ships inside button.css — so asking
 * "is spinner.css imported?" would report a bug that does not exist. Asking "is
 * `.ori-spinner` present in the CSS we actually import?" is the real invariant.
 *
 * A component whose class oriUI does not define anywhere is skipped: it has no
 * block styles of its own, so there is nothing to import for it.
 */
import { readFileSync, readdirSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = join(dirname(fileURLToPath(import.meta.url)), '..')
const srcRoot = join(webRoot, 'src')
const cssComponents = join(webRoot, '..', '..', 'node_modules', '@oriui', 'css', 'dist', 'components')

/** Every file under src/, recursively. */
function walk(dir) {
    return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
        const full = join(dir, entry.name)
        return entry.isDirectory() ? walk(full) : [full]
    })
}

/** `OriToolbarButton` -> `ori-toolbar-button` */
function toClass(component) {
    return component
        .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
        .replace(/([A-Z])([A-Z][a-z])/g, '$1-$2')
        .toLowerCase()
}

const sources = walk(srcRoot).filter((f) => /\.(vue|ts)$/.test(f))

// Components the app actually renders. Import lines alone would over-report
// (a type-only import, or a name mentioned in a comment), so require a tag.
const used = new Set()
for (const file of sources) {
    const text = readFileSync(file, 'utf8')
    for (const [, name] of text.matchAll(/<(Ori[A-Z][A-Za-z]*)[\s/>]/g)) used.add(name)
}

// The stylesheets main.ts pulls in, concatenated — this is what the browser gets.
const mainTs = readFileSync(join(srcRoot, 'main.ts'), 'utf8')
const imported = [...mainTs.matchAll(/@oriui\/css\/components\/([a-z-]+)\.css/g)].map((m) => m[1])
const loadedCss = imported.map((name) => readFileSync(join(cssComponents, `${name}.css`), 'utf8')).join('\n')

// Every class oriUI defines anywhere, so an unstyled component is distinguishable
// from a component with no styles of its own.
const allCss = readdirSync(cssComponents)
    .filter((f) => f.endsWith('.css'))
    .map((f) => readFileSync(join(cssComponents, f), 'utf8'))
    .join('\n')

/**
 * Is `.cls` present as a whole class? A boundary check, not a substring one:
 * the dist CSS is minified, so the same class turns up as `.ori-badge,`,
 * `.ori-badge{`, `.ori-badge:hover` and `.ori-badge>*`. The lookahead also stops
 * `.ori-badge` from matching inside `.ori-badge-anchor`, which is a different block.
 */
function hasClass(css, cls) {
    return new RegExp(`\.${cls}(?![\w-])`).test(css)
}

const missing = []
for (const component of [...used].sort()) {
    const cls = toClass(component)
    // A class oriui defines nowhere means the component has no block styles of its
    // own, so there is nothing to import. Note this abstains rather than
    // false-positives, and it is the one blind spot: a component whose block name
    // does not follow from its component name would be skipped silently. None of
    // the components this app renders are in that position today.
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
