import { test, expect, type Page } from '@playwright/test'

/**
 * Geometry guard for the editor shell's floating chrome.
 *
 * The shell parks its islands in six absolutely-positioned regions around the
 * canvas. Absolute positioning means nothing pushes anything else out of the
 * way: two islands can be painted on top of each other and every other gate in
 * this repo still passes. vue-tsc sees types, vitest renders into happy-dom
 * (no layout at all), stylelint reads declarations, and axe reads the
 * accessibility tree — none of them has a pixel to look at. That blind spot is
 * how this overlap bug survived: the zoom island sat on top of the toolbar across the
 * ENTIRE 601-1180px band (measured 9580px^2 at 768x1024) because the lift that
 * moves it out of the way was keyed to the phone breakpoint, while the toolbar
 * stays wide enough to reach it up to ~1169px.
 *
 * So this suite asserts one thing, in a real browser, at the widths where the
 * layout actually changes its mind: NO two bottom islands intersect.
 *
 * Deliberately generic — it compares the regions' rendered CONTENT, not
 * `/draw`'s own class names, so it keeps working when a view renames its
 * islands, and it covers pairs nobody has thought about yet (the bottom-left
 * coordinate readout only exists once the pointer has been over the canvas;
 * when it is absent it is skipped rather than faked).
 *
 * `/draw` is the probe because it is the one route that renders the full shell
 * without a session or a live match. `/play` and `/practice` slot into the very
 * same regions with the same CSS, so a fix here is a fix there.
 *
 * Run: `npm run test:layout -w @justpaint/web` (needs the Vite dev server; the
 * Playwright config starts or reuses one on :7777).
 */

/**
 * Widths where the chrome changes shape, plus one on each side of every
 * threshold. 600/601 is the phone breakpoint — the toolbar switches from its
 * compact form (290px) to its full one (769px) there, which is exactly what
 * made the old lift threshold wrong. 1200/1201 is the lift threshold itself.
 * The landscape phone is in the table because it is a viewport people actually
 * hold and nobody designs against.
 */
const VIEWPORTS = [
    { name: 'phone portrait', width: 375, height: 812 },
    { name: 'phone breakpoint', width: 600, height: 900 },
    { name: 'just past the phone breakpoint', width: 601, height: 900 },
    { name: 'phone landscape', width: 667, height: 375 },
    { name: 'tablet portrait', width: 768, height: 1024 },
    { name: 'small laptop', width: 840, height: 700 },
    { name: 'tablet landscape', width: 1024, height: 768 },
    { name: 'just under the lift threshold', width: 1150, height: 800 },
    { name: 'the lift threshold', width: 1200, height: 800 },
    { name: 'just past the lift threshold', width: 1201, height: 800 },
    { name: 'desktop', width: 1280, height: 800 }
]

/** The bottom row — the three regions that share one edge and can collide. */
const BOTTOM_REGIONS = ['bottom-left', 'bottom-center', 'bottom-right'] as const

type Box = { left: number; top: number; right: number; bottom: number }

/**
 * The rendered box of a region's CONTENT, not the region wrapper. The centre
 * region is a full-width strip (it centres its child without an offset
 * transform — see EditorShell), so measuring the wrapper would report an
 * overlap with everything on the row and mean nothing. `null` when the region
 * is absent or renders nothing.
 */
async function contentBox(page: Page, region: string): Promise<Box | null> {
    return page.evaluate((name) => {
        const wrapper = document.querySelector(`.shell__region--${name}`)
        const el = wrapper?.firstElementChild ?? wrapper
        if (!el) return null
        const r = el.getBoundingClientRect()
        return r.width && r.height ? { left: r.left, top: r.top, right: r.right, bottom: r.bottom } : null
    }, region)
}

/** Area shared by two boxes, in px². Zero means they do not touch. */
function intersectionArea(a: Box, b: Box): number {
    const w = Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left))
    const h = Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top))
    return Math.round(w * h)
}

for (const viewport of VIEWPORTS) {
    test(`bottom chrome does not overlap itself at ${viewport.width}x${viewport.height} (${viewport.name})`, async ({
        page
    }) => {
        await page.setViewportSize({ width: viewport.width, height: viewport.height })
        await page.goto('/draw')

        // The toolbar is the last of the three to settle (it carries the tool
        // buttons); waiting on it means every box below is final.
        await page.locator('.shell__region--bottom-center').first().waitFor({ state: 'visible' })

        const boxes = new Map<string, Box>()
        for (const region of BOTTOM_REGIONS) {
            const box = await contentBox(page, region)
            if (box) boxes.set(region, box)
        }

        // Two islands have to exist for the assertion to mean anything; if the
        // shell ever stops rendering them this should fail loudly rather than
        // pass by vacuum.
        expect(
            boxes.size,
            `expected at least two bottom islands to be rendered, saw ${[...boxes.keys()].join(', ') || 'none'}`
        ).toBeGreaterThanOrEqual(2)

        const regions = [...boxes.entries()]
        for (let i = 0; i < regions.length; i++) {
            for (let j = i + 1; j < regions.length; j++) {
                const [nameA, boxA] = regions[i]
                const [nameB, boxB] = regions[j]
                const area = intersectionArea(boxA, boxB)
                expect(
                    area,
                    `${nameA} and ${nameB} overlap by ${area}px² at ${viewport.width}x${viewport.height} — ` +
                        `${nameA}=${JSON.stringify(boxA)} ${nameB}=${JSON.stringify(boxB)}`
                ).toBe(0)
            }
        }
    })
}
