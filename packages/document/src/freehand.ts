import type { BrushOptions } from "./types";

/**
 * Minimal shape of perfect-freehand's `getStroke()` options. Declared locally so
 * this contract package stays dependency-free; the renderer passes the result to
 * the pinned perfect-freehand.
 */
export interface FreehandStrokeOptions {
  size: number;
  thinning: number;
  smoothing: number;
  streamline: number;
  simulatePressure: boolean;
  easing: (t: number) => number;
  last: boolean;
  start: { cap: boolean; taper: number; easing: (t: number) => number };
  end: { cap: boolean; taper: number; easing: (t: number) => number };
}

const linear = (t: number): number => t;

/**
 * Build the FULL `getStroke()` options from the curated {@link BrushOptions},
 * applying the pinned constants (§5.3) every renderer must share: rounded caps,
 * linear easings, `last: true`. The editor preview and the server render worker
 * both call `getStroke` via this function so their outlines stay byte-aligned.
 *
 * The editor MUST NOT vary a pinned option without first adding it to
 * `BrushOptions` and bumping the render contract — otherwise preview and worker
 * diverge silently.
 */
export function toFreehandOptions(brush: BrushOptions): FreehandStrokeOptions {
  return {
    size: brush.size,
    thinning: brush.thinning,
    smoothing: brush.smoothing,
    streamline: brush.streamline,
    simulatePressure: brush.simulatePressure,
    easing: linear,
    last: true,
    start: { cap: true, taper: brush.taperStart, easing: linear },
    end: { cap: true, taper: brush.taperEnd, easing: linear },
  };
}
