import type { Document, FreehandPoint, Point, Stroke } from "./types";
import { COORD_DP, PRESSURE_DP } from "./constants";
import { DocumentValidationError, validateDocument } from "./validate";

/**
 * Parse + validate a document from JSON text or an already-parsed value. Throws
 * {@link DocumentValidationError} on any structural problem. This is the
 * client-side read/write guard; the Go server re-validates authoritatively.
 */
export function parseDocument(input: string | unknown): Document {
  let value: unknown = input;
  if (typeof input === "string") {
    try {
      value = JSON.parse(input);
    } catch (e) {
      throw new DocumentValidationError(`malformed JSON: ${(e as Error).message}`);
    }
  }
  return validateDocument(value);
}

/**
 * Serialize a document to its canonical jsonb payload, rounding coordinates to
 * the write-precision (§2): geometry to 2 dp, freehand pressure to 3 dp.
 */
export function serializeDocument(doc: Document): string {
  return JSON.stringify(roundDocument(doc));
}

function round(n: number, dp: number): number {
  const f = 10 ** dp;
  return Math.round(n * f) / f;
}

function roundOpt(v: number | undefined): number | undefined {
  return v === undefined ? undefined : round(v, COORD_DP);
}

function roundPoint([x, y]: Point): Point {
  return [round(x, COORD_DP), round(y, COORD_DP)];
}

function roundStroke(s: Stroke): Stroke {
  switch (s.type) {
    case "freehand":
      return {
        ...s,
        points: s.points.map(
          ([x, y, p]): FreehandPoint => [round(x, COORD_DP), round(y, COORD_DP), round(p, PRESSURE_DP)],
        ),
      };
    case "line":
      return { ...s, points: s.points.map(roundPoint), strokeWidth: round(s.strokeWidth, COORD_DP) };
    case "polygon":
      return { ...s, points: s.points.map(roundPoint), strokeWidth: roundOpt(s.strokeWidth) };
    case "rect":
      return {
        ...s,
        x: round(s.x, COORD_DP),
        y: round(s.y, COORD_DP),
        width: round(s.width, COORD_DP),
        height: round(s.height, COORD_DP),
        cornerRadius: roundOpt(s.cornerRadius),
        strokeWidth: roundOpt(s.strokeWidth),
      };
    case "ellipse":
      return {
        ...s,
        cx: round(s.cx, COORD_DP),
        cy: round(s.cy, COORD_DP),
        rx: round(s.rx, COORD_DP),
        ry: round(s.ry, COORD_DP),
        strokeWidth: roundOpt(s.strokeWidth),
      };
  }
}

/** Return a copy of `doc` with all coordinates rounded to write-precision. */
export function roundDocument(doc: Document): Document {
  return {
    ...doc,
    layers: doc.layers.map((layer) => ({
      ...layer,
      strokes: layer.strokes.map(roundStroke),
    })),
  };
}
