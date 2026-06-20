import { BRUSH_DEFAULTS, validateDocument } from "@justpaint/document";
import type { Document } from "@justpaint/document";
import { describe, expect, it } from "vitest";
// Import the tool DIRECTLY (not the barrel) so the test never pulls in Konva.
import { ellipseTool } from "../src/tools/ellipse";
import type { LogicalPoint, ToolContext } from "../src/types";

/** Minimal ToolContext stub: fixed style + deterministic id. */
const ctx: ToolContext = {
  style: { color: "#1b1b1b", fill: "#cfe8ff", strokeWidth: 4, brush: BRUSH_DEFAULTS },
  newId: () => "s1",
};

const p = (x: number, y: number): LogicalPoint => ({ x, y, pressure: 0.5 });

/** Wrap a stroke in a one-layer document for validateDocument. */
function wrap(stroke: NonNullable<ReturnType<typeof ellipseTool.buildStroke>>): Document {
  return {
    version: 1,
    width: 100,
    height: 100,
    background: null,
    layers: [{ id: "L", name: "L", visible: true, opacity: 1, strokes: [stroke] }],
  };
}

describe("ellipseTool", () => {
  it("builds the expected ellipse from a drag bbox and validates (A)", () => {
    // Drag from (10,20) to (50,80): w=40, h=60.
    const stroke = ellipseTool.buildStroke(ctx, [p(10, 20), p(30, 40), p(50, 80)]);
    expect(stroke).not.toBeNull();
    if (stroke === null) return;

    expect(stroke.type).toBe("ellipse");
    if (stroke.type !== "ellipse") return;

    expect(stroke.id).toBe("s1");
    expect(stroke.composite).toBe("source-over");
    expect(stroke.cx).toBe(30); // 10 + 40/2
    expect(stroke.cy).toBe(50); // 20 + 60/2
    expect(stroke.rx).toBe(20); // |40|/2
    expect(stroke.ry).toBe(30); // |60|/2
    expect(stroke.fill).toBe("#cfe8ff");
    expect(stroke.stroke).toBe("#1b1b1b");
    expect(stroke.strokeWidth).toBe(4);

    // Negative drag direction still yields positive radii and centered cx/cy.
    const flipped = ellipseTool.buildStroke(ctx, [p(50, 80), p(10, 20)]);
    expect(flipped).not.toBeNull();
    if (flipped !== null && flipped.type === "ellipse") {
      expect(flipped.cx).toBe(30);
      expect(flipped.cy).toBe(50);
      expect(flipped.rx).toBe(20);
      expect(flipped.ry).toBe(30);
    }

    expect(() => validateDocument(wrap(stroke))).not.toThrow();
  });

  it("returns null for a degenerate gesture (B)", () => {
    // Same x → w=0 → rx=0 → degenerate.
    expect(ellipseTool.buildStroke(ctx, [p(40, 20), p(40, 80)])).toBeNull();
    // Same y → h=0 → ry=0 → degenerate.
    expect(ellipseTool.buildStroke(ctx, [p(20, 40), p(80, 40)])).toBeNull();
    // Single point → first === last → both radii 0 → degenerate.
    expect(ellipseTool.buildStroke(ctx, [p(40, 40)])).toBeNull();
    // Empty gesture → null.
    expect(ellipseTool.buildStroke(ctx, [])).toBeNull();
  });
});
