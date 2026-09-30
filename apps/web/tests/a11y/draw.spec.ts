import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import type { Result } from 'axe-core'

/**
 * Rendered a11y audit of /draw via axe-core in a real Chromium — catches
 * painted contrast and rendered-tree issues the happy-dom unit tests and the
 * static token lint can't see. Covers desktop/mobile and the two overlays a
 * guest can open (side menu, shortcuts dialog). Fails only on serious/critical
 * violations; lower-impact findings are logged.
 */

const BLOCKING_IMPACTS = new Set(['serious', 'critical'])

/** axe can't analyze the raster <canvas> the Konva stage renders to, and it carries no DOM semantics anyway. */
const CANVAS_SELECTOR = '.konvajs-content'

/**
 * Elements excluded per-element, not per-rule, because their color-contrast
 * finding needs a design decision, not a code fix — a brand color or an
 * oriui-owned token. color-contrast stays enabled everywhere else. Add an
 * entry only for a triaged, deliberate choice, never to silence a real bug.
 */
const AUDIT_EXCLUSIONS: { selector: string; rule: string; reason: string }[] = []

/** Navigate to /draw and wait for the editor shell (toolbar + Konva canvas) to mount. */
async function gotoDraw(page: Page): Promise<void> {
    await page.goto('/draw')
    await page.locator('.bar').waitFor({ state: 'visible' })
    await page.locator('.konvajs-content canvas').first().waitFor({ state: 'visible' })
}

/** Run axe over the current page, excluding the canvas and the documented allowlist. */
async function auditPage(page: Page) {
    let builder = new AxeBuilder({ page }).exclude(CANVAS_SELECTOR)
    for (const { selector } of AUDIT_EXCLUSIONS) {
        builder = builder.exclude(selector)
    }
    return builder.analyze()
}

function formatViolations(violations: Result[]): string {
    if (violations.length === 0) return 'no violations'
    return violations
        .map((v) => {
            const targets = v.nodes.map((n) => n.target.join(' ')).join('\n      ')
            return `  [${v.impact}] ${v.id} — ${v.help}\n    ${v.helpUrl}\n    targets:\n      ${targets}`
        })
        .join('\n')
}

async function expectNoSeriousViolations(page: Page, label: string): Promise<void> {
    const results = await auditPage(page)
    const blocking = results.violations.filter((v) => BLOCKING_IMPACTS.has(v.impact ?? ''))
    const nonBlocking = results.violations.filter((v) => !BLOCKING_IMPACTS.has(v.impact ?? ''))

    if (nonBlocking.length > 0) {
        console.log(`\n[a11y:${label}] non-blocking (moderate/minor) findings:\n${formatViolations(nonBlocking)}`)
    }

    expect(blocking, `[a11y:${label}] serious/critical violations:\n${formatViolations(blocking)}`).toEqual([])
}

test.describe('/draw — resting editor', () => {
    test('desktop (1280x800) has no serious/critical a11y violations', async ({ page }) => {
        await gotoDraw(page)
        await expectNoSeriousViolations(page, 'desktop')
    })

    test.describe('mobile viewport', () => {
        test.use({ viewport: { width: 405, height: 880 } })

        test('mobile (405x880) has no serious/critical a11y violations', async ({ page }) => {
            await gotoDraw(page)
            await expectNoSeriousViolations(page, 'mobile')
        })
    })
})

test.describe('/draw — open overlays (desktop)', () => {
    test('side menu open has no serious/critical a11y violations', async ({ page }) => {
        await gotoDraw(page)
        await page.locator('.draw__menu-toggle').click()
        // The menu is always mounted; it slides in and drops `inert` when opened.
        await page.locator('aside.menu.menu--open').waitFor({ state: 'visible' })
        await expect(page.locator('aside.menu')).toHaveJSProperty('inert', false)
        // Wait for the slide-in to settle before axe reads composited colors.
        await expect(page.locator('aside.menu')).toHaveCSS('opacity', '1')
        await expectNoSeriousViolations(page, 'menu-open')
    })

    test('shortcuts dialog open has no serious/critical a11y violations', async ({ page }) => {
        await gotoDraw(page)
        // "?" toggles the cheat-sheet (desktop only — suppressed <=600px).
        await page.keyboard.press('Shift+Slash')
        // OriDialog names its <dialog> via aria-labelledby, so match by
        // accessible name (getByRole resolves it) rather than aria-label.
        const shortcutsDialog = page.getByRole('dialog', { name: 'Keyboard shortcuts' })
        await shortcutsDialog.waitFor({ state: 'visible' })
        // Wait for the fade-in to settle — axe reading a mid-fade composite
        // flaked color-contrast before.
        await expect(shortcutsDialog).toHaveCSS('opacity', '1')
        await expectNoSeriousViolations(page, 'shortcuts-open')
    })

    test('sign-in dialog open has no serious/critical a11y violations', async ({ page }) => {
        await gotoDraw(page)
        // Click through the menu's real "Sign in" row (not the store) so this
        // exercises what a visitor actually reaches.
        await page.locator('.draw__menu-toggle').click()
        await page.getByRole('button', { name: 'Sign in' }).click()
        const signInDialog = page.getByRole('dialog', { name: 'Sign in' })
        await signInDialog.waitFor({ state: 'visible' })
        await expect(signInDialog).toHaveCSS('opacity', '1')
        await expectNoSeriousViolations(page, 'sign-in-open')
    })
})
