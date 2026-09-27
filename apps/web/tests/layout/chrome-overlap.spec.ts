import { test, expect, type Page } from '@playwright/test'

/**
 * Geometry guard for the editor shell's floating chrome: absolutely-positioned
 * islands can overlap with every other gate green (docs/NOTES.md, "Only a
 * rendered browser catches overlapping chrome"), so this suite asserts one
 * invariant, in a real browser, at the widths where the layout changes shape:
 * no two bottom islands intersect.
 *
 * Deliberately generic (compares rendered CONTENT, not `/draw`'s class names)
 * so it also catches pairs nobody expected; `/draw` is the probe because
 * `/play` and `/practice` share the same shell and CSS.
 *
 * Run: `npm run test:layout -w @justpaint/web` (needs the Vite dev server).
 */

/**
 * Widths bracketing each threshold: 600/601 is the phone breakpoint (toolbar
 * goes compact 290px -> full 769px), 1200/1201 is the zoom-island lift
 * threshold. Landscape phone is included because people hold it that way.
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
 * A region's CONTENT box, not its wrapper — the centre region is a full-width
 * strip (see EditorShell), so the wrapper would "overlap" everything on the
 * row and the check would mean nothing. `null` when the region renders nothing.
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

        // The toolbar is the last of the three to settle, so waiting on it
        // means every box below is final.
        await page.locator('.shell__region--bottom-center').first().waitFor({ state: 'visible' })

        const boxes = new Map<string, Box>()
        for (const region of BOTTOM_REGIONS) {
            const box = await contentBox(page, region)
            if (box) boxes.set(region, box)
        }

        // Fail loudly if fewer than two islands render, rather than pass by vacuum.
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
