import { describe, expect, it } from "vitest";
import type { Document } from "../src/index";
import {
  DocumentValidationError,
  parseDocument,
  roundDocument,
  serializeDocument,
} from "../src/index";

const doc: Document = {
  version: 1,
  width: 1920,
  height: 1080,
  background: "#ffffff",
  layers: [
    {
      id: "lyr",
      name: "Layer 1",
      visible: true,
      opacity: 1,
      strokes: [
        {
          id: "pen",
          type: "freehand",
          composite: "source-over",
          color: "#1b1b1b",
          points: [
            [420.123456, 300.987654, 0.426666],
            [432.5, 305.1, 0.55],
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
        {
          id: "box",
          type: "rect",
          composite: "source-over",
          x: 10.111,
          y: 20.999,
          width: 100.5,
          height: 60.25,
          fill: "#cfe8ff",
        },
      ],
    },
  ],
  meta: { generator: "justpaint-test", freehandVersion: "1.2.4" },
};

describe("serialize / parse round-trip", () => {
  it("round-trips through canonical JSON (equal to the rounded document)", () => {
    const back = parseDocument(serializeDocument(doc));
    expect(back).toEqual(roundDocument(doc));
  });

  it("parses an already-parsed object too", () => {
    expect(() => parseDocument(JSON.parse(serializeDocument(doc)))).not.toThrow();
  });

  it("rounds geometry to 2 dp and pressure to 3 dp on write", () => {
    const json = JSON.parse(serializeDocument(doc)) as Document;
    const pen = json.layers[0]!.strokes[0]!;
    if (pen.type !== "freehand") throw new Error("expected freehand");
    expect(pen.points[0]).toEqual([420.12, 300.99, 0.427]);

    const box = json.layers[0]!.strokes[1]!;
    if (box.type !== "rect") throw new Error("expected rect");
    expect(box.x).toBe(10.11);
    expect(box.y).toBe(21);
  });

  it("drops absent optional channels from the payload", () => {
    const json = JSON.parse(serializeDocument(doc)) as Record<string, unknown>;
    const box = (json.layers as Document["layers"])[0]!.strokes[1]!;
    expect("stroke" in box).toBe(false);
    expect("strokeWidth" in box).toBe(false);
  });

  it("throws DocumentValidationError on malformed JSON", () => {
    expect(() => parseDocument("{ not json")).toThrow(DocumentValidationError);
  });

  it("throws DocumentValidationError on a structurally invalid document", () => {
    expect(() => parseDocument({ version: 1, width: 0, height: 10, background: null, layers: [] })).toThrow(
      DocumentValidationError,
    );
  });
});
