#!/usr/bin/env node
/**
 * Headless Node render worker — the authoritative judged raster (trust boundary,
 * docs/GAME.md §6, DOCUMENT-FORMAT.md §10). Reads a validated vector document as
 * JSON on stdin and writes the rendered PNG as base64 on stdout (text-safe
 * across platforms). The Go server spawns this per submission
 * (server/internal/render.NodeRenderer).
 *
 * Shares the editor's projection + fit path (`renderToStage`) so the judged
 * raster matches the editor preview (docs/NOTES.md). The frame is pinned by the
 * judge contract: square 1024², opaque white background (JUDGE.md §5).
 * `import 'konva/canvas-backend'` must precede any Konva use (docs/NOTES.md).
 */
import 'konva/canvas-backend'
import { renderToStage } from '@justpaint/editor'

const JUDGE_FRAME = 1024 // square edge (JUDGE.md §5)
const JUDGE_BG = '#ffffff' // opaque white, overrides doc.background

function readStdin() {
    return new Promise((resolve, reject) => {
        const chunks = []
        process.stdin.on('data', (c) => chunks.push(c))
        process.stdin.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')))
        process.stdin.on('error', reject)
    })
}

// Local to the CLI: this module runs main() on load, so it is never imported —
// spawned as a subprocess only (server/internal/render.NodeRenderer, selftest).
function renderDocumentToPngBase64(doc) {
    const stage = renderToStage(doc, {
        outWidth: JUDGE_FRAME,
        outHeight: JUDGE_FRAME,
        fit: 'contain',
        background: JUDGE_BG
    })
    try {
        // node-canvas path (browser uses stage.toBlob; here toDataURL → base64 PNG).
        const url = stage.toDataURL({ mimeType: 'image/png', pixelRatio: 1 })
        return url.slice(url.indexOf(',') + 1)
    } finally {
        stage.destroy() // Konva keeps stages in a module registry until destroyed.
    }
}

async function main() {
    const raw = await readStdin()
    if (!raw.trim()) throw new Error('empty document on stdin')
    const doc = JSON.parse(raw)
    process.stdout.write(renderDocumentToPngBase64(doc))
}

main().catch((e) => {
    process.stderr.write(String(e?.stack ?? e) + '\n')
    process.exit(1)
})
