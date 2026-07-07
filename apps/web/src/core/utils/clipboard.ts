/**
 * Thin async wrappers over the browser Clipboard API. Both rethrow the
 * underlying `DOMException` as the `cause` of a friendlier `Error` — the
 * Clipboard API requires a secure context (https/localhost) and, in most
 * browsers, a fresh user gesture.
 */

/** Write plain text to the system clipboard. */
export async function copyText(text: string): Promise<void> {
    try {
        await navigator.clipboard.writeText(text)
    } catch (err) {
        throw new Error('Could not copy to the clipboard (needs a secure context and a user gesture).', { cause: err })
    }
}

/** Write an image blob (e.g. `image/png`) to the system clipboard. */
export async function copyImage(blob: Blob): Promise<void> {
    try {
        await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })])
    } catch (err) {
        throw new Error('Could not copy the image to the clipboard (needs a secure context and a user gesture).', {
            cause: err
        })
    }
}
