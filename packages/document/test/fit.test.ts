import { describe, expect, it } from "vitest";
import { computeFitTransform } from "../src/index";

describe("computeFitTransform (contain)", () => {
  it("is identity for an equal-size frame", () => {
    const t = computeFitTransform(1024, 1024, 1024, 1024);
    expect(t.scale).toBe(1);
    expect(t.dx).toBe(0);
    expect(t.dy).toBe(0);
  });

  it("letterboxes a landscape canvas into a square frame (bars top/bottom)", () => {
    const t = computeFitTransform(1920, 1080, 1024, 1024);
    expect(t.scale).toBeCloseTo(1024 / 1920, 10);
    expect(t.dx).toBeCloseTo(0, 10);
    expect(t.dy).toBeCloseTo((1024 - 1080 * (1024 / 1920)) / 2, 10);
    expect(t.dy).toBeGreaterThan(0);
  });

  it("pillarboxes a portrait canvas into a square frame (bars left/right)", () => {
    const t = computeFitTransform(1080, 1920, 1024, 1024);
    expect(t.scale).toBeCloseTo(1024 / 1920, 10);
    expect(t.dy).toBeCloseTo(0, 10);
    expect(t.dx).toBeGreaterThan(0);
  });

  it("preserves aspect ratio (single uniform scale)", () => {
    const w = 800;
    const h = 600;
    const t = computeFitTransform(w, h, 400, 400);
    // contained: the scaled canvas fits within the frame on both axes.
    expect(w * t.scale).toBeLessThanOrEqual(400 + 1e-9);
    expect(h * t.scale).toBeLessThanOrEqual(400 + 1e-9);
    // and touches it on the binding axis (width here).
    expect(w * t.scale).toBeCloseTo(400, 10);
  });

  it("does not round the offsets", () => {
    const t = computeFitTransform(1000, 333, 256, 256);
    expect(Number.isInteger(t.dy)).toBe(false);
  });
});
