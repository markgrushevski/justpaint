/**
 * The vector document (v1): types, constants and the helpers every renderer shares.
 * Spec: docs/DOCUMENT-FORMAT.md.
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
    StrokeType
} from './types'

export {
    BRUSH_DEFAULTS,
    COORD_DP,
    DEFAULT_BACKGROUND,
    DEFAULT_CANVAS,
    DOC_VERSION,
    FREEHAND_VERSION,
    LIMITS,
    PRESSURE_DP
} from './constants'

export { roundDocument } from './round'

export { computeFitTransform } from './fit'
export type { FitTransform } from './fit'

export { toFreehandOptions } from './freehand'
