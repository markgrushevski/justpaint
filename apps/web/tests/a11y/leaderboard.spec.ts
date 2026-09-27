import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import type { Result } from 'axe-core'

/**
 * Rendered a11y audit of /leaderboard via axe in a real Chromium: the semantic
 * table, the signed-in player's aria-current row, and roles/names as painted,
 * across desktop and mobile. Runs once the loading skeleton clears, whichever
 * terminal state (data, error, empty) the page resolves to.
 *
 * Fails only on serious/critical violations, mirroring draw.spec.ts.
 */

const BLOCKING_IMPACTS = new Set(['serious', 'critical'])

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
 * Wait for real data rows (not the aria-hidden skeletons) or a state message,
 * so the audit is deterministic whether or not the backend is serving the ladder.
 */
async function gotoLeaderboard(page: Page): Promise<void> {
    await page.goto('/leaderboard')
    await page.locator('.lb__panel').waitFor({ state: 'visible' })
    await page
        .locator('.lb__state, .lb__table tbody .lb__row:not([aria-hidden="true"])')
        .first()
        .waitFor({ state: 'visible' })
}

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
