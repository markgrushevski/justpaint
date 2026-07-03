#!/usr/bin/env node
/**
 * Headless Node render worker — the authoritative judged raster (trust boundary,
 * docs/GAME.md §6, DOCUMENT-FORMAT §10). Reads a validated vector document as JSON
 * on stdin and writes the rendered PNG as base64 on stdout (base64 keeps the pipe
 * text-safe across platforms). The Go server spawns this per submission
 * (server/internal/render.NodeRenderer).
 *
 * It shares the editor's EXACT projection + fit path (`renderToStage` from
 * `@justpaint/editor`), so the judged raster matches the editor preview — no
 * second renderer to drift (docs/DECISIONS "one shared renderer"). The frame is
 * pinned by the judge contract: square 1024², opaque white background (JUDGE.md
 * §5). `import 'konva/canvas-backend'` MUST precede any Konva use — it registers
 * the node-canvas backend (Konva 10 dropped the default Node backend).
 */
import "konva/canvas-backend";
import { renderToStage } from "@justpaint/editor";

const JUDGE_FRAME = 1024; // square edge (JUDGE.md §5)
const JUDGE_BG = "#ffffff"; // opaque white, overrides doc.background

function readStdin() {
  return new Promise((resolve, reject) => {
    const chunks = [];
    process.stdin.on("data", (c) => chunks.push(c));
    process.stdin.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    process.stdin.on("error", reject);
  });
}

// Local to the CLI: this module runs main() on load, so it is never imported —
// spawned as a subprocess only (server/internal/render.NodeRenderer, selftest).
function renderDocumentToPngBase64(doc) {
  const stage = renderToStage(doc, {
    outWidth: JUDGE_FRAME,
    outHeight: JUDGE_FRAME,
    fit: "contain",
    background: JUDGE_BG,
  });
  try {
    // node-canvas path (browser uses stage.toBlob; here toDataURL → base64 PNG).
    const url = stage.toDataURL({ mimeType: "image/png", pixelRatio: 1 });
    return url.slice(url.indexOf(",") + 1);
  } finally {
    stage.destroy(); // Konva keeps stages in a module registry until destroyed.
  }
}

async function main() {
  const raw = await readStdin();
  if (!raw.trim()) throw new Error("empty document on stdin");
  const doc = JSON.parse(raw);
  process.stdout.write(renderDocumentToPngBase64(doc));
}

main().catch((e) => {
  process.stderr.write(String(e?.stack ?? e) + "\n");
  process.exit(1);
});
