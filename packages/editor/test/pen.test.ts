import { BRUSH_DEFAULTS, validateDocument } from "@justpaint/document";
import { describe, expect, it } from "vitest";
import { penTool } from "../src/tools/pen";
import type { LogicalPoint, ToolContext } from "../src/types";

/** A minimal ToolContext stub — fixed style + deterministic id. */
const ctx: ToolContext = {
  style: {
    color: "#1b1b1b",
    fill: "#cfe8ff",
    strokeWidth: 4,
    brush: BRUSH_DEFAULTS,
  },
  newId: () => "s1",
};

/** Wrap a stroke in a minimal one-layer document for validation. */
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

describe("penTool", () => {
  it("id is pen", () => {
    expect(penTool.id).toBe("pen");
  });

  // Test A: a normal multi-point gesture builds the expected freehand stroke,
  // and the produced stroke passes validateDocument inside a one-layer document.
  it("builds a freehand source-over stroke from all gesture points", () => {
    const gesture: LogicalPoint[] = [
      { x: 10, y: 20, pressure: 0.4 },
      { x: 12.5, y: 24.1, pressure: 0.55 },
      { x: 30, y: 40, pressure: 0.61 },
    ];

    const stroke = penTool.buildStroke(ctx, gesture);
    expect(stroke).not.toBeNull();
    if (stroke === null) throw new Error("unreachable");

    expect(stroke.type).toBe("freehand");
    if (stroke.type !== "freehand") throw new Error("unreachable");

    expect(stroke.id).toBe("s1");
    expect(stroke.composite).toBe("source-over");
    expect(stroke.color).toBe("#1b1b1b");
    expect(stroke.brush).toBe(BRUSH_DEFAULTS);

    // points = gesture.map(p => [p.x, p.y, p.pressure]) — raw, unrounded.
    expect(stroke.points).toEqual([
      [10, 20, 0.4],
      [12.5, 24.1, 0.55],
      [30, 40, 0.61],
    ]);

    expect(() => validateDocument(wrap(stroke))).not.toThrow();
  });

  // Test B (pen variant): pen never returns null — a single-point gesture
  // yields a valid 1-point freehand stroke (a dot).
  it("treats a single-point gesture as a valid 1-point dot", () => {
    const gesture: LogicalPoint[] = [{ x: 50, y: 50, pressure: 0.5 }];

    const stroke = penTool.buildStroke(ctx, gesture);
    expect(stroke).not.toBeNull();
    if (stroke === null) throw new Error("unreachable");
    if (stroke.type !== "freehand") throw new Error("unreachable");

    expect(stroke.points).toEqual([[50, 50, 0.5]]);
    expect(stroke.points).toHaveLength(1);

    expect(() => validateDocument(wrap(stroke))).not.toThrow();
  });
});
