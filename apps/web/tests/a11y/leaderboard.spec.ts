import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import type { Result } from 'axe-core'

/**
 * Rendered-a11y audit of the leaderboard page (`/leaderboard`) — the app's first
 * plain (non-editor) page and its first cached read. axe drives a real Chromium
 * so this sees the semantic <table> (caption + column headers), the signed-in
 * player's highlighted `aria-current` row, and the back-to-drawing link as
 * painted (contrast / roles / accessible names), across a desktop and a mobile
 * viewport.
 *
 * The page renders whichever of its states resolves — the data table when the
 * API is up, or the error / empty message otherwise. The audit runs once the
 * loading skeletons have cleared, so it always covers the real terminal render.
 *
 * Bar: ZERO violations of impact `serious` or `critical` (mirrors draw.spec.ts).
 * Lower-impact findings are printed for visibility but don't fail.
 */

const BLOCKING_IMPACTS = new Set(['serious', 'critical'])

/** Human-readable dump of violations for the assertion message. */
function formatViolations(violations: Result[]): string {
    if (violations.length === 0) return 'no violations'
    return violations
        .map((v) => {
            const targets = v.nodes.map((n) => n.target.join(' ')).join('\n      ')
            return `  [${v.impact}] ${v.id} — ${v.help}\n    ${v.helpUrl}\n    targets:\n      ${targets}`
        })
        .join('\n')
}

/**
 * Navigate to /leaderboard and wait for the island + a settled body: either real
 * data rows (`.lb__row` that are NOT the `aria-hidden` skeletons) or a state
 * message (error / empty). This makes the audit deterministic whether or not the
 * backend is serving the ladder.
 */
async function gotoLeaderboard(page: Page): Promise<void> {
    await page.goto('/leaderboard')
    await page.locator('.lb__panel').waitFor({ state: 'visible' })
    await page
        .locator('.lb__state, .lb__table tbody .lb__row:not([aria-hidden="true"])')
        .first()
        .waitFor({ state: 'visible' })
}

/** Assert the page is free of serious/critical violations; log the rest. */
async function expectNoSeriousViolations(page: Page, label: string): Promise<void> {
    const results = await new AxeBuilder({ page }).analyze()
    const blocking = results.violations.filter((v) => BLOCKING_IMPACTS.has(v.impact ?? ''))
    const nonBlocking = results.violations.filter((v) => !BLOCKING_IMPACTS.has(v.impact ?? ''))

    if (nonBlocking.length > 0) {
        console.log(`\n[a11y:${label}] non-blocking (moderate/minor) findings:\n${formatViolations(nonBlocking)}`)
    }

    expect(blocking, `[a11y:${label}] serious/critical violations:\n${formatViolations(blocking)}`).toEqual([])
}

test.describe('/leaderboard', () => {
    test('desktop (1280x800) has no serious/critical a11y violations', async ({ page }) => {
        await gotoLeaderboard(page)
        await expectNoSeriousViolations(page, 'leaderboard-desktop')
    })

    test.describe('mobile viewport', () => {
        test.use({ viewport: { width: 405, height: 880 } })

        test('mobile (405x880) has no serious/critical a11y violations', async ({ page }) => {
            await gotoLeaderboard(page)
            await expectNoSeriousViolations(page, 'leaderboard-mobile')
        })
    })
})
