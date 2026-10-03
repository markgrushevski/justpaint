import { test, expect, type Page } from '@playwright/test'

/**
 * The /draw menu on a phone: a modal drawer from the right edge (OriDrawer), named after the
 * drawing. Esc steps back from a sub-panel before it closes the drawer, a colour picker's
 * popover closes on its own, and a tap on the strip of canvas beside the drawer closes it.
 * Guest only, so the API answers 401.
 *
 * Run: `npm run test:flows -w @justpaint/web` (needs the Vite dev server).
 */

test.use({ viewport: { width: 390, height: 844 }, hasTouch: true })

async function openMenu(page: Page) {
    // A predicate, not a glob: `**/api/**` would catch the app's own `src/core/api` modules.
    await page.route(
        (url) => url.pathname.startsWith('/api/'),
        (route) => route.fulfill({ status: 401, json: { error: { code: 'unauthorized', message: 'unauthorized' } } })
    )
    await page.goto('/draw')
    await page.locator('.konvajs-content canvas').first().waitFor({ state: 'visible' })
    await page.getByRole('button', { name: 'Open menu', exact: true }).click()
    const drawer = page.getByRole('dialog', { name: 'Unsaved drawing' })
    await expect(drawer).toBeVisible()
    return drawer
}

const isModal = (page: Page) => page.evaluate(() => document.querySelector('dialog:modal') !== null)

test.describe('/draw menu on a phone', () => {
    test('opens as a modal drawer with focus on the first row', async ({ page }) => {
        await openMenu(page)
        expect(await isModal(page)).toBe(true)
        await expect(page.getByRole('button', { name: 'New drawing' })).toBeFocused()
        // No keyboard on a phone, so no shortcuts row.
        await expect(page.getByRole('button', { name: 'Keyboard shortcuts' })).toHaveCount(0)
    })

    test('Esc steps back from a sub-panel, then closes', async ({ page }) => {
        const drawer = await openMenu(page)
        await page.getByRole('button', { name: 'Export' }).click()
        await expect(page.getByRole('button', { name: 'Download PNG' })).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(drawer).toBeVisible()
        await expect(page.getByRole('button', { name: 'New drawing' })).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(drawer).toBeHidden()
        await expect(page.getByRole('button', { name: 'Open menu', exact: true })).toBeVisible()
    })

    test('Esc in the colour picker closes only the picker', async ({ page }) => {
        const drawer = await openMenu(page)
        await page.getByRole('button', { name: 'Custom canvas colour' }).click()
        const picker = page.getByRole('dialog', { name: 'Custom canvas colour' })
        await expect(picker).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(picker).toBeHidden()
        await expect(drawer).toBeVisible()
    })

    test('a tap beside the drawer closes it', async ({ page }) => {
        const drawer = await openMenu(page)
        const box = await drawer.boundingBox()
        expect(box).not.toBeNull()
        await page.mouse.click((box?.x ?? 40) / 2, 400)
        await expect(drawer).toBeHidden()
    })
})
