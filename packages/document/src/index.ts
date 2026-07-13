/**
 * `@justpaint/document` — the canonical vector-document contract (v1).
 *
 * One schema shared by three consumers: the editor (render + edit, Konva), the
 * Go backend (store as jsonb, validate), and the judge (render → PNG to score).
 * See `docs/DOCUMENT-FORMAT.md` for the spec.
 */
export type {
  BBox,
  BrushOptions,
  Color,
  Composite,
  DocSummary,
  DocVersion,
  Document,
  DocumentMeta,
  EllipseStroke,
  FreehandPoint,
  FreehandStroke,
  Id,
  Layer,
  LineCap,
  LineJoin,
  LineStroke,
  Op,
  OpStroke,
  OpStrokeType,
  Point,
  PolygonStroke,
  RectStroke,
  Stroke,
  StrokeBase,
  StrokeType,
} from "./types";

export {
  BRUSH_DEFAULTS,
  COORD_DP,
  DEFAULT_BACKGROUND,
  DEFAULT_CANVAS,
  DOC_VERSION,
  FREEHAND_VERSION,
  LIMITS,
  PRESSURE_DP,
} from "./constants";

export {
  DocumentValidationError,
  safeValidateDocument,
  safeValidateOpBatch,
  validateDocument,
  validateOpBatch,
} from "./validate";
export type { OpValidationResult, ValidationResult } from "./validate";

export { parseDocument, roundDocument, serializeDocument } from "./parse";

export { computeFitTransform } from "./fit";
export type { FitTransform } from "./fit";

export { toFreehandOptions } from "./freehand";
export type { FreehandStrokeOptions } from "./freehand";
