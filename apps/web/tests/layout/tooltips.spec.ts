import { test, expect, type Page } from '@playwright/test'

/**
 * Every tooltip bubble sits at its own trigger. oriui pairs a bubble with its trigger through
 * a CSS anchor name all tooltips share, and the browser resolves a shared name to the last
 * eligible trigger unless the name is scoped, so a bubble could open across the screen
 * (docs/NOTES.md). Measured with the layers panel and the menu open, which add the most
 * triggers; a hidden bubble keeps its box, so nothing has to be hovered.
 *
 * Run: `npm run test:layout -w @justpaint/web` (needs the Vite dev server).
 */

interface Misplaced {
    text: string
    offset: number
    gap: number
}

/** Bubbles whose centre is off their trigger's, unless held in by the screen edge, or that float away from it. */
async function misplaced(page: Page): Promise<{ count: number; bad: Misplaced[] }> {
    return page.evaluate(() => {
        const bad: { text: string; offset: number; gap: number }[] = []
        let count = 0
        for (const tip of document.querySelectorAll('.ori-tooltip')) {
            const trigger = tip.querySelector('.ori-tooltip__trigger')?.getBoundingClientRect()
            const bubble = tip.querySelector('.ori-tooltip__bubble')
            if (!trigger?.width || !bubble) continue
            count++
            const r = bubble.getBoundingClientRect()
            const offset = Math.abs(r.left + r.width / 2 - (trigger.left + trigger.width / 2))
            const atEdge = r.left <= 1 || r.right >= window.innerWidth - 1
            const gap = Math.max(r.top - trigger.bottom, trigger.top - r.bottom)
            if ((offset > 2 && !atEdge) || gap > 16) {
                bad.push({ text: bubble.textContent?.trim() ?? '', offset: Math.round(offset), gap: Math.round(gap) })
            }
        }
        return { count, bad }
    })
}

test('every tooltip opens at its own trigger', async ({ page }) => {
    await page.goto('/draw')
    await page.locator('.konvajs-content canvas').first().waitFor({ state: 'visible' })
    await page.getByRole('button', { name: 'Layers', exact: true }).click()
    await page.getByRole('button', { name: 'Open menu', exact: true }).click()
    await expect(page.locator('aside.menu')).toHaveCSS('opacity', '1')

    const { count, bad } = await misplaced(page)
    expect(count).toBeGreaterThan(20)
    expect(bad).toEqual([])
})
