import { describe, expect, it } from "vitest";
import type { DocSummary } from "../src/index";
import { LIMITS, validateOpBatch } from "../src/index";

// AI-assist op-batch contract (docs/ASSIST.md §2). This table is mirrored 1:1 —
// identical case names — by server/internal/document/ops_test.go. A schema change
// lands in both validators AND both test tables together (keystone parity).

/** A minimal summary with the given layer ids (each named after its id, 0 strokes). */
function summaryWith(...layerIds: string[]): DocSummary {
  return {
    canvas: { width: 100, height: 100 },
    layers: layerIds.map((id) => ({ id, name: id, strokeCount: 0 })),
  };
}

/** A one-op `add_stroke` batch placing `stroke` on `layerId` (mirrors docWith). */
function opsWith(layerId: string, stroke: unknown): unknown[] {
  return [{ kind: "add_stroke", layerId, stroke }];
}

/** A valid non-freehand stroke of the given id (default: a unit rect). */
const rect = (id: string) => ({
  id,
  type: "rect",
  composite: "source-over",
  x: 0,
  y: 0,
  width: 10,
  height: 10,
  fill: "#000000",
});

const freehand = (id: string) => ({
  id,
  type: "freehand",
  composite: "source-over",
  color: "#000000",
  points: [[1, 1, 0.5]],
  brush: { size: 1, thinning: 0, smoothing: 0, streamline: 0, simulatePressure: false, taperStart: 0, taperEnd: 0 },
});

describe("validateOpBatch", () => {
  const valid: Array<[string, DocSummary, unknown]> = [
    ["add_stroke onto existing summary layer", summaryWith("L1"), opsWith("L1", rect("s1"))],
    [
      "add_layer then add_stroke referencing it",
      summaryWith(),
      [
        { kind: "add_layer", id: "new", name: "New" },
        { kind: "add_stroke", layerId: "new", stroke: rect("s1") },
      ],
    ],
    ["empty batch", summaryWith("L1"), []],
  ];

  const overBatchCap = Array.from(
    { length: LIMITS.maxOpsPerBatch + 1 },
    (_v, i) => ({ kind: "add_layer", id: `L${i}`, name: "n" }),
  );

  const invalid: Array<[string, DocSummary, unknown]> = [
    ["dangling add_stroke layerId", summaryWith("L1"), opsWith("nope", rect("s1"))],
    [
      "forward ref: stroke before its layer",
      summaryWith(),
      [
        { kind: "add_stroke", layerId: "new", stroke: rect("s1") },
        { kind: "add_layer", id: "new", name: "New" },
      ],
    ],
    ["freehand in add_stroke", summaryWith("L1"), opsWith("L1", freehand("s1"))],
    ["op id collides with summary id", summaryWith("L1"), [{ kind: "add_layer", id: "L1", name: "dup" }]],
    [
      "two ops share an id",
      summaryWith(),
      [
        { kind: "add_layer", id: "dup", name: "A" },
        { kind: "add_layer", id: "dup", name: "B" },
      ],
    ],
    ["add_stroke stroke id collides with summary layer id", summaryWith("L1"), opsWith("L1", rect("L1"))],
    ["add_stroke zero-area rect", summaryWith("L1"), opsWith("L1", { id: "s1", type: "rect", composite: "source-over", x: 0, y: 0, width: 0, height: 10, fill: "#000000" })],
    ["add_stroke polygon under three vertices", summaryWith("L1"), opsWith("L1", { id: "s1", type: "polygon", composite: "source-over", points: [[0, 0], [10, 10]], fill: "#000000" })],
    ["batch over maxOpsPerBatch", summaryWith(), overBatchCap],
    ["add_layer empty name", summaryWith(), [{ kind: "add_layer", id: "x", name: "" }]],
    ["add_layer name too long", summaryWith(), [{ kind: "add_layer", id: "x", name: "x".repeat(LIMITS.maxNameLen + 1) }]],
    ["unknown op kind", summaryWith(), [{ kind: "delete_layer", id: "x" }]],
    // required keys must be present — mirrored 1:1 with the Go table's presence guard.
    ["missing required key: add_layer name", summaryWith(), [{ kind: "add_layer", id: "x" }]],
    ["missing required key: add_stroke layerId", summaryWith("L1"), [{ kind: "add_stroke", stroke: rect("s1") }]],
  ];

  it.each(valid)("accepts: %s", (_name, summary, ops) => {
    expect(() => validateOpBatch(summary, ops)).not.toThrow();
  });

  it.each(invalid)("rejects: %s", (_name, summary, ops) => {
    expect(() => validateOpBatch(summary, ops)).toThrow();
  });

  it("reports a path on the offending op", () => {
    const ops = opsWith("L1", { id: "s1", type: "ellipse", composite: "source-over", cx: 1, cy: 1, rx: 0, ry: 1 });
    try {
      validateOpBatch(summaryWith("L1"), ops);
      expect.unreachable("should have thrown");
    } catch (e) {
      expect((e as { path?: string }).path).toContain("ops[0].stroke");
    }
  });
});
