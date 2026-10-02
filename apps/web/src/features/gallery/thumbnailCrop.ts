/**
 * The window a gallery preview is cut from: the drawn content, padded and grown to the preview's
 * aspect so a small sketch fills its card. Pure, so it needs neither Konva nor a DOM to test.
 */

export const THUMB_WIDTH = 400
export const THUMB_HEIGHT = 300

export interface Rect {
    x: number
    y: number
    width: number
    height: number
}

const ASPECT = THUMB_WIDTH / THUMB_HEIGHT

/** Margin around the content: a share of its larger side, never less than PAD_MIN document units. */
const PAD_RATIO = 0.08
const PAD_MIN = 16

/** Narrowest window, as a share of the document width: a lone dot stays a dot, not a pixelated blob. */
const MIN_WIDTH_RATIO = 0.3

/** The part of `bounds` inside the document frame, since layers clip to it; null when nothing is left. */
function insideFrame(bounds: Rect, docWidth: number, docHeight: number): Rect | null {
    const x0 = Math.max(bounds.x, 0)
    const y0 = Math.max(bounds.y, 0)
    const x1 = Math.min(bounds.x + bounds.width, docWidth)
    const y1 = Math.min(bounds.y + bounds.height, docHeight)
    return x1 > x0 && y1 > y0 ? { x: x0, y: y0, width: x1 - x0, height: y1 - y0 } : null
}

/** Where a window of `size` starts along one axis: at `start`, kept inside `[0, frame]`, or centered on the frame if it is larger. */
function place(start: number, size: number, frame: number): number {
    if (size >= frame) return (frame - size) / 2
    return Math.min(Math.max(start, 0), frame - size)
}

/**
 * The crop, in document units and at the preview's aspect, for content spanning `bounds`
 * (null for an empty drawing, which shows the whole frame letterboxed).
 */
export function thumbnailCrop(bounds: Rect | null, docWidth: number, docHeight: number): Rect {
    const content = bounds ? insideFrame(bounds, docWidth, docHeight) : null

    if (!content) {
        const width = Math.max(docWidth, docHeight * ASPECT)
        const height = width / ASPECT
        return { x: place(0, width, docWidth), y: place(0, height, docHeight), width, height }
    }

    const pad = Math.max(PAD_MIN, PAD_RATIO * Math.max(content.width, content.height))
    let width = content.width + 2 * pad
    let height = content.height + 2 * pad
    if (width / height < ASPECT) width = height * ASPECT
    else height = width / ASPECT

    if (width < MIN_WIDTH_RATIO * docWidth) {
        width = MIN_WIDTH_RATIO * docWidth
        height = width / ASPECT
    }

    const centerX = content.x + content.width / 2
    const centerY = content.y + content.height / 2
    return {
        x: place(centerX - width / 2, width, docWidth),
        y: place(centerY - height / 2, height, docHeight),
        width,
        height
    }
}
