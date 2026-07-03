import { describe, expect, it } from "vitest";
import {
  clampZoom,
  fitView,
  MAX_ZOOM,
  MIN_ZOOM,
  panBy,
  zoomAround,
  zoomTo,
  type ViewState,
} from "../src/view";

describe("clampZoom", () => {
  it("clamps to [MIN_ZOOM, MAX_ZOOM] and rejects non-finite", () => {
    expect(clampZoom(0.001)).toBe(MIN_ZOOM);
    expect(clampZoom(1000)).toBe(MAX_ZOOM);
    expect(clampZoom(1)).toBe(1);
    // Non-finite input (NaN / ±Infinity) falls back to the safe default 1.
    expect(clampZoom(Number.NaN)).toBe(1);
    expect(clampZoom(Infinity)).toBe(1);
  });
});

describe("fitView", () => {
  it("scales a landscape doc to fit width and centers vertically", () => {
    // 1000x500 doc into a 500x500 viewport → scale 0.5, letterbox top/bottom.
    const v = fitView(1000, 500, 500, 500);
    expect(v.zoom).toBeCloseTo(0.5);
    expect(v.panX).toBeCloseTo(0);
    expect(v.panY).toBeCloseTo((500 - 500 * 0.5) / 2); // 125
  });

  it("scales a portrait doc to fit height and centers horizontally", () => {
    const v = fitView(500, 1000, 500, 500);
    expect(v.zoom).toBeCloseTo(0.5);
    expect(v.panY).toBeCloseTo(0);
    expect(v.panX).toBeCloseTo(125);
  });

  it("returns an identity view for a degenerate viewport", () => {
    expect(fitView(100, 100, 0, 0)).toEqual({ zoom: 1, panX: 0, panY: 0 });
  });
});

describe("zoomAround", () => {
  const base: ViewState = { zoom: 1, panX: 0, panY: 0 };

  it("keeps the logical point under the anchor fixed", () => {
    // Anchor at screen (200,100); the logical point under it must not move.
    const logicalBefore = { x: (200 - base.panX) / base.zoom, y: (100 - base.panY) / base.zoom };
    const v = zoomAround(base, 2, 200, 100);
    expect(v.zoom).toBe(2);
    const screenAfter = { x: logicalBefore.x * v.zoom + v.panX, y: logicalBefore.y * v.zoom + v.panY };
    expect(screenAfter.x).toBeCloseTo(200);
    expect(screenAfter.y).toBeCloseTo(100);
  });

  it("clamps at MAX_ZOOM and still anchors exactly (applied factor uses the clamped zoom)", () => {
    const v = zoomAround({ zoom: MAX_ZOOM / 1.5, panX: 10, panY: 20 }, 4, 300, 300);
    expect(v.zoom).toBe(MAX_ZOOM);
    // logical point under (300,300) is unchanged
    const start = { zoom: MAX_ZOOM / 1.5, panX: 10, panY: 20 };
    const lx = (300 - start.panX) / start.zoom;
    expect(lx * v.zoom + v.panX).toBeCloseTo(300);
  });

  it("zoomTo reaches the target zoom anchored at the point", () => {
    const v = zoomTo(base, 4, 50, 50);
    expect(v.zoom).toBe(4);
    expect((50 - v.panX) / v.zoom).toBeCloseTo((50 - base.panX) / base.zoom);
  });
});

describe("panBy", () => {
  it("translates pan without touching zoom", () => {
    expect(panBy({ zoom: 2, panX: 5, panY: 7 }, 10, -3)).toEqual({ zoom: 2, panX: 15, panY: 4 });
  });
});
