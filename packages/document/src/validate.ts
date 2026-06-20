import type {
  Composite,
  Document,
  LineCap,
  LineJoin,
  StrokeType,
} from "./types";
import { LIMITS } from "./constants";

/**
 * A client-fixable problem with a document (maps to HTTP 400 on the server).
 * `path` locates the offending node, e.g. `layers[0].strokes[2].points[3]`.
 */
export class DocumentValidationError extends Error {
  readonly path: string;
  constructor(message: string, path = "") {
    super(path ? `${path}: ${message}` : message);
    this.name = "DocumentValidationError";
    this.path = path;
  }
}

const HEX_COLOR = /^#([0-9a-f]{6}|[0-9a-f]{8})$/;
const STROKE_TYPES: readonly StrokeType[] = [
  "freehand",
  "line",
  "rect",
  "ellipse",
  "polygon",
];
const CAPS: readonly LineCap[] = ["butt", "round", "square"];
const JOINS: readonly LineJoin[] = ["miter", "round", "bevel"];

type Obj = Record<string, unknown>;

function fail(path: string, msg: string): never {
  throw new DocumentValidationError(msg, path);
}

function isObject(v: unknown): v is Obj {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isFiniteNumber(v: unknown): v is number {
  return typeof v === "number" && Number.isFinite(v);
}

/** Code-point length, matching the Go validator's `utf8.RuneCountInString`. */
function runeLen(s: string): number {
  return [...s].length;
}

function checkColor(value: unknown, path: string): void {
  if (typeof value !== "string" || !HEX_COLOR.test(value)) {
    fail(path, "invalid color (want #rrggbb or #rrggbbaa)");
  }
}

function checkId(value: unknown, path: string, seen: Set<string>): void {
  if (typeof value !== "string" || value.length < 1 || runeLen(value) > LIMITS.maxIdLen) {
    fail(path, `id must be 1-${LIMITS.maxIdLen} chars`);
  }
  if (seen.has(value)) fail(path, `duplicate id "${value}"`);
  seen.add(value);
}

/**
 * Validate a 2-tuple / 3-tuple point array and return its length. `arity` is the
 * required tuple width; `min` the minimum number of points.
 */
function checkPointArray(
  value: unknown,
  path: string,
  arity: number,
  min: number,
): number {
  if (!Array.isArray(value)) fail(path, "must be an array");
  if (value.length < min) {
    fail(path, `needs at least ${min} point${min > 1 ? "s" : ""}`);
  }
  if (value.length > LIMITS.maxPointsPerStroke) {
    fail(path, `too many points (max ${LIMITS.maxPointsPerStroke})`);
  }
  value.forEach((p, i) => {
    const pp = `${path}[${i}]`;
    if (!Array.isArray(p) || p.length !== arity) fail(pp, `must be a ${arity}-tuple`);
    for (const n of p) if (!isFiniteNumber(n)) fail(pp, "non-finite point");
  });
  return value.length;
}

/** Shared optional fill/stroke channels for shapes (rect / ellipse / polygon). */
function checkFillStroke(v: Obj, path: string): void {
  if (v.fill !== undefined && v.fill !== null) checkColor(v.fill, `${path}.fill`);
  if (v.stroke !== undefined && v.stroke !== null) {
    checkColor(v.stroke, `${path}.stroke`);
    if (v.strokeWidth !== undefined && (!isFiniteNumber(v.strokeWidth) || v.strokeWidth <= 0)) {
      fail(`${path}.strokeWidth`, "must be > 0 when stroke is present");
    }
  }
}

function validateBrush(value: unknown, path: string): void {
  if (!isObject(value)) fail(path, "brush must be an object");
  const b = value;
  if (!isFiniteNumber(b.size) || b.size < 0) fail(`${path}.size`, "must be >= 0");
  if (!isFiniteNumber(b.thinning) || b.thinning < -1 || b.thinning > 1) {
    fail(`${path}.thinning`, "must be in [-1,1]");
  }
  if (!isFiniteNumber(b.smoothing) || b.smoothing < 0 || b.smoothing > 1) {
    fail(`${path}.smoothing`, "must be in [0,1]");
  }
  if (!isFiniteNumber(b.streamline) || b.streamline < 0 || b.streamline > 1) {
    fail(`${path}.streamline`, "must be in [0,1]");
  }
  if (typeof b.simulatePressure !== "boolean") {
    fail(`${path}.simulatePressure`, "must be a boolean");
  }
  if (!isFiniteNumber(b.taperStart) || b.taperStart < 0) fail(`${path}.taperStart`, "must be >= 0");
  if (!isFiniteNumber(b.taperEnd) || b.taperEnd < 0) fail(`${path}.taperEnd`, "must be >= 0");
}

function validateFreehand(v: Obj, path: string): number {
  checkColor(v.color, `${path}.color`);
  const points = v.points;
  if (!Array.isArray(points)) fail(`${path}.points`, "must be an array");
  if (points.length < 1) fail(`${path}.points`, "freehand needs at least 1 point");
  if (points.length > LIMITS.maxPointsPerStroke) {
    fail(`${path}.points`, `too many points (max ${LIMITS.maxPointsPerStroke})`);
  }
  points.forEach((p, i) => {
    const pp = `${path}.points[${i}]`;
    if (!Array.isArray(p) || p.length !== 3) fail(pp, "must be [x,y,pressure]");
    const [x, y, pressure] = p as unknown[];
    if (!isFiniteNumber(x) || !isFiniteNumber(y) || !isFiniteNumber(pressure)) {
      fail(pp, "non-finite point");
    }
    if (pressure < 0 || pressure > 1) fail(pp, "pressure must be in [0,1]");
  });
  validateBrush(v.brush, `${path}.brush`);
  return points.length;
}

function validateLine(v: Obj, path: string): number {
  const n = checkPointArray(v.points, `${path}.points`, 2, 2);
  checkColor(v.stroke, `${path}.stroke`);
  if (!isFiniteNumber(v.strokeWidth) || v.strokeWidth <= 0) {
    fail(`${path}.strokeWidth`, "must be > 0");
  }
  if (v.cap !== undefined && !CAPS.includes(v.cap as LineCap)) fail(`${path}.cap`, "invalid cap");
  if (v.join !== undefined && !JOINS.includes(v.join as LineJoin)) fail(`${path}.join`, "invalid join");
  return n;
}

function validatePolygon(v: Obj, path: string): number {
  const n = checkPointArray(v.points, `${path}.points`, 2, 3);
  if (v.join !== undefined && !JOINS.includes(v.join as LineJoin)) fail(`${path}.join`, "invalid join");
  checkFillStroke(v, path);
  return n;
}

function validateRect(v: Obj, path: string): void {
  for (const k of ["x", "y", "width", "height"] as const) {
    if (!isFiniteNumber(v[k])) fail(`${path}.${k}`, "non-finite geometry");
  }
  if ((v.width as number) <= 0 || (v.height as number) <= 0) {
    fail(path, "width and height must be > 0 (zero-area rejected)");
  }
  if (v.cornerRadius !== undefined && (!isFiniteNumber(v.cornerRadius) || v.cornerRadius < 0)) {
    fail(`${path}.cornerRadius`, "must be >= 0");
  }
  checkFillStroke(v, path);
}

function validateEllipse(v: Obj, path: string): void {
  for (const k of ["cx", "cy", "rx", "ry"] as const) {
    if (!isFiniteNumber(v[k])) fail(`${path}.${k}`, "non-finite geometry");
  }
  if ((v.rx as number) <= 0 || (v.ry as number) <= 0) fail(path, "rx and ry must be > 0");
  checkFillStroke(v, path);
}

/** Validate one stroke; returns its input-point count (0 for rect/ellipse). */
function validateStroke(value: unknown, path: string, ids: Set<string>): number {
  if (!isObject(value)) fail(path, "stroke must be an object");
  checkId(value.id, `${path}.id`, ids);
  if (typeof value.type !== "string" || !STROKE_TYPES.includes(value.type as StrokeType)) {
    fail(`${path}.type`, `unknown stroke type ${JSON.stringify(value.type)}`);
  }
  const composite = value.composite as Composite;
  if (composite !== "source-over" && composite !== "destination-out") {
    fail(`${path}.composite`, `invalid composite ${JSON.stringify(value.composite)}`);
  }
  // `bbox` is advisory (§8) — tolerated and ignored here.
  switch (value.type as StrokeType) {
    case "freehand":
      return validateFreehand(value, path);
    case "line":
      return validateLine(value, path);
    case "polygon":
      return validatePolygon(value, path);
    case "rect":
      validateRect(value, path);
      return 0;
    case "ellipse":
      validateEllipse(value, path);
      return 0;
  }
}

function validateLayer(value: unknown, path: string, ids: Set<string>): unknown[] {
  if (!isObject(value)) fail(path, "layer must be an object");
  checkId(value.id, `${path}.id`, ids);
  if (
    typeof value.name !== "string" ||
    runeLen(value.name) < 1 ||
    runeLen(value.name) > LIMITS.maxNameLen
  ) {
    fail(`${path}.name`, `must be 1-${LIMITS.maxNameLen} chars`);
  }
  if (typeof value.visible !== "boolean") fail(`${path}.visible`, "must be a boolean");
  if (!isFiniteNumber(value.opacity) || value.opacity < 0 || value.opacity > 1) {
    fail(`${path}.opacity`, "must be in [0,1]");
  }
  if (!Array.isArray(value.strokes)) fail(`${path}.strokes`, "must be an array");
  return value.strokes;
}

/**
 * Validate an untrusted value against the v1 schema. Returns it typed as a
 * {@link Document} on success, throws {@link DocumentValidationError} otherwise.
 * Mirrors the Go write-edge validator; the server re-validates authoritatively.
 * Unknown fields are tolerated (forward-compat).
 */
export function validateDocument(value: unknown): Document {
  if (!isObject(value)) fail("", "document must be an object");

  if (value.version !== 1) fail("version", `unknown document version ${String(value.version)}`);

  for (const k of ["width", "height"] as const) {
    const n = value[k];
    if (!isFiniteNumber(n) || !Number.isInteger(n) || n < 1 || n > LIMITS.maxCanvasDimension) {
      fail(k, `must be an integer in [1, ${LIMITS.maxCanvasDimension}]`);
    }
  }

  if (value.background !== null) {
    if (typeof value.background !== "string" || !HEX_COLOR.test(value.background)) {
      fail("background", "must be null, #rrggbb or #rrggbbaa");
    }
  }

  const layers = value.layers;
  if (!Array.isArray(layers)) fail("layers", "must be an array");
  if (layers.length < 1) fail("layers", "document must have at least one layer");
  if (layers.length > LIMITS.maxLayers) fail("layers", `too many layers (max ${LIMITS.maxLayers})`);

  const ids = new Set<string>(); // single id namespace across layers + strokes
  let totalStrokes = 0;
  let totalPoints = 0;

  layers.forEach((layer, li) => {
    const lp = `layers[${li}]`;
    const strokes = validateLayer(layer, lp, ids);
    totalStrokes += strokes.length;
    if (totalStrokes > LIMITS.maxStrokes) {
      fail(`${lp}.strokes`, `too many strokes (max ${LIMITS.maxStrokes})`);
    }
    strokes.forEach((stroke, si) => {
      totalPoints += validateStroke(stroke, `${lp}.strokes[${si}]`, ids);
      if (totalPoints > LIMITS.maxTotalPoints) {
        fail(lp, `too many total points (max ${LIMITS.maxTotalPoints})`);
      }
    });
  });

  return value as unknown as Document;
}

export type ValidationResult =
  | { ok: true; document: Document }
  | { ok: false; error: DocumentValidationError };

/** Non-throwing wrapper around {@link validateDocument}. */
export function safeValidateDocument(value: unknown): ValidationResult {
  try {
    return { ok: true, document: validateDocument(value) };
  } catch (e) {
    if (e instanceof DocumentValidationError) return { ok: false, error: e };
    throw e;
  }
}
