/**
 * Pure viewport math for the editor's zoom/pan — the "fit to viewport" surface
 * (ROADMAP Phase 2). Kept side-effect-free and DOM-free so it's unit-testable;
 * the {@link Editor} applies the result to the Konva **stage** (size + scale +
 * position), never a CSS transform, so `getRelativePointerPosition` keeps
 * returning logical document coordinates (DOCUMENT-FORMAT §2 / NOTES).
 */
import { computeFitTransform } from "@justpaint/document";

/** Zoom + pan of the logical document within the viewport (stage) frame. */
export interface ViewState {
  /** Uniform scale: screen pixels per logical unit. */
  zoom: number;
  /** Stage position (screen px) — where the logical origin (0,0) lands. */
  panX: number;
  panY: number;
}

export const MIN_ZOOM = 0.02;
export const MAX_ZOOM = 64;
/** Multiplicative step for the zoom in/out buttons. */
export const ZOOM_STEP = 1.2;

export function clampZoom(zoom: number): number {
  if (!Number.isFinite(zoom)) return 1;
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom));
}

/**
 * Scale-to-fit + center the `docW × docH` logical canvas into a `vpW × vpH`
 * viewport. Reuses the pinned `computeFitTransform` (the same contain math the
 * PNG renderer uses), so on-screen framing matches an exported render.
 */
export function fitView(docW: number, docH: number, vpW: number, vpH: number): ViewState {
  if (vpW <= 0 || vpH <= 0 || docW <= 0 || docH <= 0) {
    return { zoom: 1, panX: 0, panY: 0 };
  }
  const { scale, dx, dy } = computeFitTransform(docW, docH, vpW, vpH);
  return { zoom: clampZoom(scale), panX: dx, panY: dy };
}

/**
 * Multiply the zoom by `factor`, keeping the logical point currently under the
 * screen point `(centerX, centerY)` fixed (zoom toward the cursor). Pan is
 * adjusted so that anchor point doesn't drift.
 */
export function zoomAround(
  view: ViewState,
  factor: number,
  centerX: number,
  centerY: number,
): ViewState {
  const zoom = clampZoom(view.zoom * factor);
  // The applied factor after clamping (so the anchor math stays exact at limits).
  const applied = zoom / view.zoom;
  return {
    zoom,
    panX: centerX - (centerX - view.panX) * applied,
    panY: centerY - (centerY - view.panY) * applied,
  };
}

/** Set an absolute zoom, anchored at `(centerX, centerY)`. */
export function zoomTo(
  view: ViewState,
  zoom: number,
  centerX: number,
  centerY: number,
): ViewState {
  const target = clampZoom(zoom);
  return zoomAround(view, target / view.zoom, centerX, centerY);
}

/** Translate the view by a screen-space delta (a pan drag). */
export function panBy(view: ViewState, dx: number, dy: number): ViewState {
  return { zoom: view.zoom, panX: view.panX + dx, panY: view.panY + dy };
}
