import { describe, expect, it } from 'vitest'
import { THUMB_HEIGHT, THUMB_WIDTH, thumbnailCrop } from './thumbnailCrop'
import type { Rect } from './thumbnailCrop'

const ASPECT = THUMB_WIDTH / THUMB_HEIGHT

function expectRect(actual: Rect, expected: Rect): void {
    expect(actual.x).toBeCloseTo(expected.x, 6)
    expect(actual.y).toBeCloseTo(expected.y, 6)
    expect(actual.width).toBeCloseTo(expected.width, 6)
    expect(actual.height).toBeCloseTo(expected.height, 6)
}

describe('thumbnailCrop', () => {
    describe('without content', () => {
        it('shows the whole frame', () => {
            expectRect(thumbnailCrop(null, 400, 300), { x: 0, y: 0, width: 400, height: 300 })
        })

        it('letterboxes a frame of another shape to the preview aspect, centered', () => {
            expectRect(thumbnailCrop(null, 300, 400), { x: -(1600 / 3 - 300) / 2, y: 0, width: 1600 / 3, height: 400 })
            expectRect(thumbnailCrop(null, 800, 200), { x: 0, y: -(600 - 200) / 2, width: 800, height: 600 })
        })

        it('treats content wholly outside the frame as none', () => {
            const outside: Rect = { x: 500, y: 20, width: 40, height: 40 }
            expectRect(thumbnailCrop(outside, 400, 300), thumbnailCrop(null, 400, 300))
        })
    })

    describe('small content', () => {
        it('is padded up to the minimum width and kept inside the frame at the top-left corner', () => {
            const crop = thumbnailCrop({ x: 10, y: 10, width: 10, height: 10 }, 400, 300)
            expectRect(crop, { x: 0, y: 0, width: 120, height: 90 })
        })

        it('is kept inside the frame at the bottom-right corner', () => {
            const crop = thumbnailCrop({ x: 380, y: 280, width: 10, height: 10 }, 400, 300)
            expectRect(crop, { x: 280, y: 210, width: 120, height: 90 })
        })

        it('is centered on the content in the middle of the frame', () => {
            const crop = thumbnailCrop({ x: 190, y: 140, width: 20, height: 20 }, 400, 300)
            expectRect(crop, { x: 140, y: 105, width: 120, height: 90 })
        })

        it('ignores the part of the content that lies outside the frame', () => {
            const crop = thumbnailCrop({ x: -100, y: -100, width: 130, height: 130 }, 400, 300)
            expectRect(crop, { x: 0, y: 0, width: 120, height: 90 })
        })
    })

    describe('medium content', () => {
        it('grows the height of wide content to the preview aspect', () => {
            const crop = thumbnailCrop({ x: 50, y: 140, width: 300, height: 20 }, 400, 300)
            // 8% of 300 = 24 of padding on every side: 348 wide, grown to 261 tall around the center.
            expectRect(crop, { x: 26, y: 19.5, width: 348, height: 261 })
        })

        it('grows the width of tall content to the preview aspect', () => {
            const crop = thumbnailCrop({ x: 180, y: 40, width: 20, height: 200 }, 400, 300)
            // 16 of padding: 232 tall, grown to 232 * 4/3 wide around the center.
            expectRect(crop, { x: 190 - (232 * ASPECT) / 2, y: 24, width: 232 * ASPECT, height: 232 })
        })
    })

    describe('content larger than the frame', () => {
        it('centers on the frame when the padded content outgrows both dimensions', () => {
            const crop = thumbnailCrop({ x: 0, y: 0, width: 400, height: 300 }, 400, 300)
            // 8% of 400 = 32 of padding: 364 tall, grown to 364 * 4/3 wide.
            const width = 364 * ASPECT
            expectRect(crop, { x: (400 - width) / 2, y: (300 - 364) / 2, width, height: 364 })
        })

        it('centers only the dimension that outgrows the frame', () => {
            const crop = thumbnailCrop({ x: 100, y: 20, width: 600, height: 160 }, 800, 200)
            // 48 of padding: 696 wide, grown to 522 tall; the height overflows, the width still fits.
            expectRect(crop, { x: 52, y: (200 - 522) / 2, width: 696, height: 522 })
        })
    })

    it('always returns a window of the preview aspect', () => {
        const samples: Array<Rect | null> = [
            null,
            { x: 5, y: 5, width: 1, height: 1 },
            { x: 0, y: 0, width: 400, height: 300 },
            { x: 100, y: 20, width: 250, height: 30 },
            { x: 300, y: 10, width: 60, height: 280 }
        ]
        for (const bounds of samples) {
            const crop = thumbnailCrop(bounds, 400, 300)
            expect(crop.width / crop.height).toBeCloseTo(ASPECT, 9)
        }
    })

    it('stays inside the frame whenever it fits', () => {
        const crop = thumbnailCrop({ x: 330, y: 20, width: 60, height: 40 }, 400, 300)
        expect(crop.x).toBeGreaterThanOrEqual(0)
        expect(crop.y).toBeGreaterThanOrEqual(0)
        expect(crop.x + crop.width).toBeLessThanOrEqual(400 + 1e-9)
        expect(crop.y + crop.height).toBeLessThanOrEqual(300 + 1e-9)
    })
})
