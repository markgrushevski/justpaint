/**
 * Smoke test for the bundled render worker, exercising the EXACT path the Go
 * server uses: spawn `node dist/render.mjs`, pipe a document to stdin, read the
 * base64 PNG from stdout. Asserts a valid PNG of the pinned 1024² judge frame
 * with real ink. Run: npm run build && npm run selftest -w @justpaint/render.
 * Not a unit suite — a fast liveness check that node-canvas + Konva headless +
 * the shared projection work here.
 */
import { spawnSync } from "node:child_process";

const doc = {
  version: 1,
  width: 1080,
  height: 1080,
  background: "#ffffff",
  layers: [
    {
      id: "l1",
      name: "Layer 1",
      visible: true,
      opacity: 1,
      strokes: [
        { id: "r1", type: "rect", composite: "source-over", x: 100, y: 100, width: 400, height: 300, fill: "#ff0000" },
        { id: "e1", type: "ellipse", composite: "source-over", cx: 700, cy: 700, rx: 200, ry: 120, fill: "#0066ff" },
        {
          id: "f1",
          type: "freehand",
          composite: "source-over",
          color: "#000000",
          points: [[150, 800, 0.5], [300, 850, 0.6], [500, 820, 0.5], [700, 900, 0.7]],
          brush: { size: 24, thinning: 0.5, smoothing: 0.5, streamline: 0.5, simulatePressure: true, taperStart: 0, taperEnd: 0 },
        },
      ],
    },
  ],
};

const res = spawnSync(process.execPath, ["dist/render.mjs"], {
  input: JSON.stringify(doc),
  maxBuffer: 64 << 20,
});
if (res.status !== 0) {
  console.error("render worker exited", res.status, "\n" + res.stderr.toString());
  process.exit(1);
}

const png = Buffer.from(res.stdout.toString(), "base64");
const okMagic = [0x89, 0x50, 0x4e, 0x47].every((b, i) => png[i] === b);
const w = png.readUInt32BE(16); // IHDR width  (big-endian uint32 @16)
const h = png.readUInt32BE(20); // IHDR height (big-endian uint32 @20)

if (!okMagic) throw new Error("output is not a PNG");
if (w !== 1024 || h !== 1024) throw new Error(`frame ${w}x${h}, want 1024x1024`);
if (png.length < 1000) throw new Error(`png suspiciously small (${png.length}b) — likely blank`);

console.log(`selftest OK: ${w}x${h} PNG, ${png.length} bytes`);
