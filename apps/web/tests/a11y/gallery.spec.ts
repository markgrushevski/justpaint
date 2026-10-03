import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import type { Result } from 'axe-core'

/**
 * Rendered a11y audit of /gallery via axe in a real Chromium, across desktop and mobile.
 * The API is mocked in the browser so the grid state is deterministic: a signed-in
 * visitor, three saved drawings, one page. Fails only on serious/critical violations,
 * mirroring draw.spec.ts.
 */

const BLOCKING_IMPACTS = new Set(['serious', 'critical'])

const USER = { id: 'u1', login: 'painter', displayName: null, rating: 1000, createdAt: '2026-09-01T10:00:00Z' }

const PEN = {
    id: 'stk1',
    type: 'freehand',
    composite: 'source-over',
    color: '#1b1b1b',
    points: [
        [80, 150, 0.5],
        [160, 90, 0.5],
        [240, 210, 0.5],
        [320, 130, 0.5]
    ],
    brush: {
        size: 16,
        thinning: 0.5,
        smoothing: 0.5,
        streamline: 0.5,
        simulatePressure: true,
        taperStart: 0,
        taperEnd: 0
    }
}

function documentWith(strokes: object[]) {
    return {
        version: 1,
        width: 400,
        height: 300,
        background: null,
        layers: [{ id: 'l1', name: 'Layer 1', visible: true, opacity: 1, strokes }]
    }
}

const FIXTURES = [
    { id: 'd1', name: 'Sunset study', strokes: [PEN], updatedAt: '2026-09-29T09:00:00Z' },
    {
        id: 'd2',
        name: 'A very long drawing name that has to be cut short by the card',
        strokes: [PEN],
        updatedAt: '2026-09-20T09:00:00Z'
    },
    { id: 'd3', name: 'Blank page', strokes: [], updatedAt: '2026-08-01T09:00:00Z' }
]

function metaOf(f: (typeof FIXTURES)[number]) {
    return {
        id: f.id,
        ownerId: USER.id,
        matchId: null,
        name: f.name,
        docVersion: 1,
        width: 400,
        height: 300,
        thumbnailUrl: null,
        createdAt: f.updatedAt,
        updatedAt: f.updatedAt
    }
}

/** Answers /api/auth/me and /api/drawings* from the fixtures above, never reaching the Go server. */
async function mockApi(page: Page): Promise<void> {
    await page.route('**/api/auth/me', (route) => route.fulfill({ json: { user: USER } }))
    await page.route(/\/api\/drawings(\/|\?|$)/, (route) => {
        const id = new URL(route.request().url()).pathname.split('/')[3]
        if (!id) {
            return route.fulfill({ json: { drawings: FIXTURES.map(metaOf), nextCursor: null, limit: 24 } })
        }
        const found = FIXTURES.find((f) => f.id === id)
        return found
            ? route.fulfill({ json: { drawing: { ...metaOf(found), document: documentWith(found.strokes) } } })
            : route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'not found' } } })
    })
}

/** Wait for every card and its rendered preview, so the audit sees the settled grid. */
async function gotoGallery(page: Page): Promise<void> {
    await mockApi(page)
    await page.goto('/gallery')
    await page.locator('.drawing').first().waitFor({ state: 'visible' })
    await expect(page.locator('.drawing')).toHaveCount(FIXTURES.length)
    await expect(page.locator('.drawing__img')).toHaveCount(FIXTURES.length)
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
    const results = await new AxeBuilder({ page }).analyze()
    const blocking = results.violations.filter((v) => BLOCKING_IMPACTS.has(v.impact ?? ''))
    const nonBlocking = results.violations.filter((v) => !BLOCKING_IMPACTS.has(v.impact ?? ''))

    if (nonBlocking.length > 0) {
        console.log(`\n[a11y:${label}] non-blocking (moderate/minor) findings:\n${formatViolations(nonBlocking)}`)
    }

    expect(blocking, `[a11y:${label}] serious/critical violations:\n${formatViolations(blocking)}`).toEqual([])
}

test.describe('/gallery', () => {
    test('desktop (1280x800) has no serious/critical a11y violations', async ({ page }) => {
        await gotoGallery(page)
        await expectNoSeriousViolations(page, 'gallery-desktop')
    })

    test.describe('mobile viewport', () => {
        test.use({ viewport: { width: 375, height: 812 } })

        test('mobile (375x812) has no serious/critical a11y violations', async ({ page }) => {
            await gotoGallery(page)
            await expectNoSeriousViolations(page, 'gallery-mobile')
        })

        test('mobile (375x812) does not scroll sideways', async ({ page }) => {
            await gotoGallery(page)
            const overflow = await page.locator('.gallery').evaluate((el) => el.scrollWidth - el.clientWidth)
            expect(overflow).toBe(0)
        })
    })
})
