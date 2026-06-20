import { describe, expect, it } from "vitest";
import { BRUSH_DEFAULTS, validateDocument } from "@justpaint/document";

// Import the tool DIRECTLY (not via the barrel) so the test never pulls Konva in.
import { eraserTool } from "../src/tools/eraser";
import type { LogicalPoint, ToolContext } from "../src/types";

/** Minimal ToolContext stub — deterministic id, fixed style. */
const ctx: ToolContext = {
  style: {
    color: "#1b1b1b",
    fill: "#cfe8ff",
    strokeWidth: 4,
    brush: BRUSH_DEFAULTS,
  },
  newId: () => "s1",
};

/** Wrap a stroke in a minimal one-layer document for schema validation. */
function wrap(stroke: unknown) {
  return {
    version: 1,
    width: 100,
    height: 100,
    background: null,
    layers: [
      { id: "L", name: "L", visible: true, opacity: 1, strokes: [stroke] },
    ],
  };
}

describe("eraserTool", () => {
  it("A: builds the expected destination-out freehand stroke that validates", () => {
    const gesture: LogicalPoint[] = [
      { x: 10, y: 20, pressure: 0.4 },
      { x: 12.5, y: 25.1, pressure: 0.6 },
      { x: 30, y: 40, pressure: 0.5 },
    ];

    const stroke = eraserTool.buildStroke(ctx, gesture);
    expect(stroke).not.toBeNull();
    // Narrowed for the assertions below.
    const s = stroke!;

    // Type + the eraser's defining trait.
    expect(s.type).toBe("freehand");
    expect(s.composite).toBe("destination-out");

    // Id + style wiring.
    expect(s.id).toBe("s1");
    expect(s.type === "freehand" && s.color).toBe("#1b1b1b"); // color set for uniformity
    expect(s.type === "freehand" && s.brush).toBe(BRUSH_DEFAULTS);

    // Key geometry: points are 3-tuples [x, y, pressure], unrounded, in order.
    expect(s.type === "freehand" && s.points).toEqual([
      [10, 20, 0.4],
      [12.5, 25.1, 0.6],
      [30, 40, 0.5],
    ]);

    // The produced stroke must pass the canonical validator inside a document.
    expect(() => validateDocument(wrap(s))).not.toThrow();
  });

  it("B: a single-point gesture yields a valid 1-point freehand stroke (never null)", () => {
    const stroke = eraserTool.buildStroke(ctx, [{ x: 5, y: 5, pressure: 0.5 }]);

    expect(stroke).not.toBeNull();
    const s = stroke!;
    expect(s.type).toBe("freehand");
    expect(s.composite).toBe("destination-out");
    expect(s.type === "freehand" && s.points).toEqual([[5, 5, 0.5]]);

    // A 1-point "dot" eraser is valid per §5.3.
    expect(() => validateDocument(wrap(s))).not.toThrow();
  });
});
