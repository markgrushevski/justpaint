import { describe, expect, it } from "vitest";
import { LIMITS, validateDocument } from "../src/index";

const validMinimal = {
  version: 1,
  width: 1920,
  height: 1080,
  background: "#ffffff",
  layers: [
    {
      id: "l1",
      name: "Layer 1",
      visible: true,
      opacity: 1,
      strokes: [
        {
          id: "s1",
          type: "freehand",
          composite: "source-over",
          color: "#1b1b1b",
          points: [
            [10, 10, 0.5],
            [20, 20, 0.6],
          ],
          brush: {
            size: 16,
            thinning: 0.5,
            smoothing: 0.5,
            streamline: 0.5,
            simulatePressure: true,
            taperStart: 0,
            taperEnd: 0,
          },
        },
      ],
    },
  ],
};

const validFull = {
  version: 1,
  width: 1920,
  height: 1080,
  background: null,
  layers: [
    {
      id: "shapes",
      name: "Shapes",
      visible: true,
      opacity: 1,
      strokes: [
        { id: "r", type: "rect", composite: "source-over", x: 200, y: 150, width: 400, height: 260, cornerRadius: 12, fill: "#cfe8ff", stroke: "#0050a0", strokeWidth: 3 },
        { id: "e", type: "ellipse", composite: "source-over", cx: 1100, cy: 400, rx: 180, ry: 120, fill: "#ffe0b3", stroke: null },
        { id: "t", type: "polygon", composite: "source-over", points: [[800, 700], [950, 950], [650, 950]], fill: "#d9ffd9", stroke: "#1b7a1b", strokeWidth: 2, join: "round" },
        { id: "ln", type: "line", composite: "source-over", points: [[100, 1000], [1820, 1000]], stroke: "#333333", strokeWidth: 4, cap: "round", join: "round" },
      ],
    },
    {
      id: "ink",
      name: "Ink",
      visible: true,
      opacity: 0.85,
      strokes: [
        { id: "pen", type: "freehand", composite: "source-over", color: "#1b1b1b", points: [[300, 200, 0.4], [340, 230, 0.6]], brush: { size: 22, thinning: 0.6, smoothing: 0.5, streamline: 0.6, simulatePressure: true, taperStart: 0, taperEnd: 12 } },
        { id: "erase", type: "freehand", composite: "destination-out", color: "#000000", points: [[380, 235, 0.9], [410, 245, 0.9]], brush: { size: 30, thinning: 0, smoothing: 0.5, streamline: 0.4, simulatePressure: false, taperStart: 0, taperEnd: 0 } },
      ],
    },
  ],
};

/** Wrap one or more stroke objects in an otherwise-valid one-layer document. */
function docWith(...strokes: unknown[]): unknown {
  return {
    version: 1,
    width: 100,
    height: 100,
    background: null,
    layers: [{ id: "L", name: "L", visible: true, opacity: 1, strokes }],
  };
}

describe("validateDocument", () => {
  const valid: Array<[string, unknown]> = [
    ["minimal", validMinimal],
    ["full tool set", validFull],
    ["unknown field tolerated (forward-compat)", { version: 1, width: 10, height: 10, background: null, future: 42, layers: [{ id: "l", name: "L", visible: true, opacity: 1, strokes: [] }] }],
    ["rect with fill only (no stroke)", docWith({ id: "s", type: "rect", composite: "source-over", x: 0, y: 0, width: 10, height: 10, fill: "#000000" })],
    ["shape stroke can erase (destination-out)", docWith({ id: "s", type: "rect", composite: "destination-out", x: 0, y: 0, width: 10, height: 10, fill: "#000000" })],
    ["explicit null background is allowed", { version: 1, width: 10, height: 10, background: null, layers: [{ id: "l", name: "L", visible: true, opacity: 1, strokes: [] }] }],
  ];

  const invalid: Array<[string, unknown]> = [
    // document level
    ["bad version", { version: 2, width: 10, height: 10, background: null, layers: [{ id: "l", name: "L", visible: true, opacity: 1, strokes: [] }] }],
    ["dimension too large", { version: 1, width: 9000, height: 10, background: null, layers: [{ id: "l", name: "L", visible: true, opacity: 1, strokes: [] }] }],
    ["dimension zero", { version: 1, width: 0, height: 10, background: null, layers: [{ id: "l", name: "L", visible: true, opacity: 1, strokes: [] }] }],
    ["non-integer dimension", { version: 1, width: 10.5, height: 10, background: null, layers: [{ id: "l", name: "L", visible: true, opacity: 1, strokes: [] }] }],
    ["no layers", { version: 1, width: 10, height: 10, background: null, layers: [] }],
    ["bad background color", { version: 1, width: 10, height: 10, background: "#xyz", layers: [{ id: "l", name: "L", visible: true, opacity: 1, strokes: [] }] }],
    ["layer opacity out of range", { version: 1, width: 10, height: 10, background: null, layers: [{ id: "l", name: "L", visible: true, opacity: 1.5, strokes: [] }] }],
    ["duplicate id", docWith({ id: "L", type: "rect", composite: "source-over", x: 0, y: 0, width: 10, height: 10, fill: "#000000" })],
    // stroke level
    ["unknown stroke type", docWith({ id: "s", type: "blob", composite: "source-over" })],
    ["bad composite", docWith({ id: "s", type: "rect", composite: "xor", x: 0, y: 0, width: 10, height: 10, fill: "#000000" })],
    ["freehand zero points", docWith({ id: "s", type: "freehand", composite: "source-over", color: "#000000", points: [], brush: { size: 1, thinning: 0, smoothing: 0, streamline: 0, simulatePressure: false, taperStart: 0, taperEnd: 0 } })],
    ["freehand pressure out of range", docWith({ id: "s", type: "freehand", composite: "source-over", color: "#000000", points: [[1, 1, 2]], brush: { size: 1, thinning: 0, smoothing: 0, streamline: 0, simulatePressure: false, taperStart: 0, taperEnd: 0 } })],
    ["line under two points", docWith({ id: "s", type: "line", composite: "source-over", points: [[0, 0]], stroke: "#000000", strokeWidth: 1 })],
    ["line strokeWidth zero", docWith({ id: "s", type: "line", composite: "source-over", points: [[0, 0], [1, 1]], stroke: "#000000", strokeWidth: 0 })],
    ["rect zero area", docWith({ id: "s", type: "rect", composite: "source-over", x: 0, y: 0, width: 0, height: 10, fill: "#000000" })],
    ["ellipse zero radius", docWith({ id: "s", type: "ellipse", composite: "source-over", cx: 5, cy: 5, rx: 0, ry: 5, fill: "#000000" })],
    ["polygon under three vertices", docWith({ id: "s", type: "polygon", composite: "source-over", points: [[0, 0], [10, 10]], fill: "#000000" })],
    ["shape strokeWidth zero with stroke present", docWith({ id: "s", type: "rect", composite: "source-over", x: 0, y: 0, width: 10, height: 10, stroke: "#000000", strokeWidth: 0 })],
    ["freehand point wrong arity (2 elems)", docWith({ id: "s", type: "freehand", composite: "source-over", color: "#000000", points: [[1, 1]], brush: { size: 1, thinning: 0, smoothing: 0, streamline: 0, simulatePressure: false, taperStart: 0, taperEnd: 0 } })],
    ["line point wrong arity (3 elems)", docWith({ id: "s", type: "line", composite: "source-over", points: [[0, 0, 0], [1, 1, 1]], stroke: "#000000", strokeWidth: 1 })],
    ["non-finite coordinate", docWith({ id: "s", type: "line", composite: "source-over", points: [[0, 0], [null, 1]], stroke: "#000000", strokeWidth: 1 })],
    // required keys must be present — mirrored 1:1 with the Go table (server/internal/document)
    // so an absent required field is rejected identically on both sides (docs/NOTES.md).
    ["missing layer visible", { version: 1, width: 10, height: 10, background: null, layers: [{ id: "l", name: "L", opacity: 1, strokes: [] }] }],
    ["missing layer opacity", { version: 1, width: 10, height: 10, background: null, layers: [{ id: "l", name: "L", visible: true, strokes: [] }] }],
    ["missing layer strokes", { version: 1, width: 10, height: 10, background: null, layers: [{ id: "l", name: "L", visible: true, opacity: 1 }] }],
    ["missing document background", { version: 1, width: 10, height: 10, layers: [{ id: "l", name: "L", visible: true, opacity: 1, strokes: [] }] }],
    ["missing freehand brush", docWith({ id: "s", type: "freehand", composite: "source-over", color: "#000000", points: [[1, 1, 0.5], [2, 2, 0.6]] })],
  ];

  it.each(valid)("accepts: %s", (_name, doc) => {
    expect(() => validateDocument(doc)).not.toThrow();
  });

  it.each(invalid)("rejects: %s", (_name, doc) => {
    expect(() => validateDocument(doc)).toThrow();
  });

  it("rejects exceeding the per-stroke point cap", () => {
    const points = Array.from({ length: LIMITS.maxPointsPerStroke + 1 }, () => [0, 0]);
    const doc = docWith({ id: "s", type: "line", composite: "source-over", points, stroke: "#000000", strokeWidth: 1 });
    expect(() => validateDocument(doc)).toThrow(/too many points/);
  });

  it("rejects exceeding the total-points cap across strokes", () => {
    const half = Math.floor(LIMITS.maxPointsPerStroke);
    const mkLine = (id: string) => ({
      id,
      type: "line",
      composite: "source-over",
      points: Array.from({ length: half }, () => [0, 0]),
      stroke: "#000000",
      strokeWidth: 1,
    });
    // 11 lines * 10k points = 110k > 100k total cap.
    const strokes = Array.from({ length: 11 }, (_v, i) => mkLine(`s${i}`));
    expect(() => validateDocument(docWith(...strokes))).toThrow(/total points/);
  });

  it("reports a path on the offending node", () => {
    const doc = docWith({ id: "s", type: "ellipse", composite: "source-over", cx: 1, cy: 1, rx: 0, ry: 1 });
    try {
      validateDocument(doc);
      expect.unreachable("should have thrown");
    } catch (e) {
      expect((e as { path?: string }).path).toContain("layers[0].strokes[0]");
    }
  });
});
