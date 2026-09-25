/**
 * Guard for the hand-maintained oriUI stylesheet list in `src/main.ts`.
 *
 * We import oriUI's CSS à la carte — one file per component actually used — which
 * keeps the bundle honest but leaves a list that a human must remember to update.
 * It was already wrong: `OriBadge` and `OriSkeleton` were rendered on /leaderboard
 * and in the judging overlay with no block styles at all, because nobody added
 * their two lines.
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
import { createRequire } from 'node:module'

const webRoot = join(dirname(fileURLToPath(import.meta.url)), '..')
const srcRoot = join(webRoot, 'src')
// Ask Node where `@oriui/css` actually is rather than guessing a path into the
// workspace root: npm hoists to the root or to `apps/web/node_modules`
// depending on what else is installed, and the day it chose the latter this
// guard died with ENOENT on a package that was present and correct.
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
 * Components whose block name does NOT follow from their component name, so the
 * derivation below cannot reach them. Verified against oriui's own templates
 * (packages/vue/src/components/toolbar), not guessed from the CSS: the two
 * toolbar controls render no eponymous block at all — they compose OriButton, so
 * the DOM carries `.ori-button` — and the separator renders a BEM element of the
 * toolbar block. Without these three the guard would ABSTAIN on them (it skips
 * anything oriui defines nowhere), which is silent non-coverage rather than a
 * false alarm — the dangerous half.
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
    const cls = BLOCK_ALIASES[component] ?? toClass(component)
    // A class oriui defines nowhere means the component has no block styles of its
    // own, so there is nothing to import. This abstains rather than false-positives,
    // which is the blind spot BLOCK_ALIASES exists to close: add an entry whenever a
    // component turns out to render a block its name does not predict.
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
