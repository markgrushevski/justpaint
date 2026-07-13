import type { BrushOptions } from "./types";

/** Current document schema version. */
export const DOC_VERSION = 1 as const;

/** Default logical canvas for a new free-draw document (§2). */
export const DEFAULT_CANVAS = { width: 1920, height: 1080 } as const;

/** Default background for a new document (§3). */
export const DEFAULT_BACKGROUND = "#ffffff";

/**
 * DoS + structural caps. These MUST stay in lockstep with the Go validator
 * (`server/internal/document/validate.go`) and `docs/API.md §6`. The binding
 * semantic cap is `maxTotalPoints`.
 */
export const LIMITS = {
  maxCanvasDimension: 8192,
  maxLayers: 64,
  maxStrokes: 5_000,
  maxPointsPerStroke: 10_000,
  maxTotalPoints: 100_000,
  maxIdLen: 64,
  maxNameLen: 64,
  /** Max ops in one AI-assist batch (docs/ASSIST.md §2). Per-batch, not per-doc. */
  maxOpsPerBatch: 64,
} as const;

/**
 * Pinned perfect-freehand version — part of the render contract (§5.3, §9).
 * Stamped into `meta.freehandVersion`; the editor preview and the server render
 * worker MUST resolve this exact version from the lockfile, or their outlines
 * diverge. Reconcile this constant with the actual installed version when
 * perfect-freehand is added to `packages/editor`.
 */
export const FREEHAND_VERSION = "1.2.3";

/** Editor/tool brush defaults (§5.3). Identical everywhere — no dev/prod drift. */
export const BRUSH_DEFAULTS: BrushOptions = {
  size: 16,
  thinning: 0.5,
  smoothing: 0.5,
  streamline: 0.5,
  simulatePressure: true,
  taperStart: 0,
  taperEnd: 0,
};

/** Write-precision (§2): geometry rounds to 2 dp, freehand pressure to 3 dp. */
export const COORD_DP = 2;
export const PRESSURE_DP = 3;
