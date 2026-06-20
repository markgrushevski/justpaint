/**
 * The pinned "contain" fit transform (docs/DOCUMENT-FORMAT.md §10).
 *
 * Part of the render contract: every renderer (editor preview, server worker,
 * judge frame) MUST use this exact math, or anti-aliased edges shift and judged
 * pixels change. `dx`/`dy` are deliberately NOT rounded — keep them fractional
 * so each rasterizer anti-aliases the same sub-pixel offset.
 */
export interface FitTransform {
  /** Uniform scale applied to logical coords. */
  scale: number;
  /** Logical-origin offset within the output frame (fractional, do not round). */
  dx: number;
  dy: number;
}

/** Scale-to-fit + center a `src` logical canvas into an `out` output frame. */
export function computeFitTransform(
  srcWidth: number,
  srcHeight: number,
  outWidth: number,
  outHeight: number,
): FitTransform {
  const scale = Math.min(outWidth / srcWidth, outHeight / srcHeight);
  const dx = (outWidth - srcWidth * scale) / 2;
  const dy = (outHeight - srcHeight * scale) / 2;
  return { scale, dx, dy };
}
