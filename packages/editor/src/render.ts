/**
 * Document → PNG, the deterministic raster export (DOCUMENT-FORMAT §10).
 *
 * BROWSER-ONLY for Phase 1: it builds a real `Konva.Stage` and calls
 * `stage.toBlob`, both of which need a DOM + `<canvas>`. The headless server
 * render worker is a later, separate path (Konva-node) — this is not it, and
 * it is NOT unit-tested (no DOM in the test runner).
 */
import Konva from "konva";
import { computeFitTransform } from "@justpaint/document";
import type { Document } from "@justpaint/document";
import { toKonva } from "./konva";

export interface RenderOptions {
  outWidth: number;
  outHeight: number;
  /** Scale-to-fit + center (the only supported mode in v1). */
  fit?: "contain";
  /** REPLACES `doc.background` when provided (undefined = use the doc's). */
  background?: string | null;
  /** Internal supersample factor; never changes the output dimensions. */
  pixelRatio?: number;
}

/**
 * Render `doc` to a fixed-size PNG blob, scaled-to-fit + centered into the
 * `outWidth × outHeight` frame using the pinned fit transform (§10).
 *
 * The stage spans the FULL output frame, so the effective background fills the
 * letterbox margins (§2). Each projected content layer carries the fit
 * transform — `translate(dx, dy)` then `scale(scale)` — applied per layer (not
 * to the stage) so the background layer stays frame-aligned. `dx`/`dy` are kept
 * fractional (never rounded) so anti-aliasing matches every other renderer.
 */
export async function renderToPNG(doc: Document, opts: RenderOptions): Promise<Blob> {
  const { scale, dx, dy } = computeFitTransform(
    doc.width,
    doc.height,
    opts.outWidth,
    opts.outHeight,
  );

  // The effective background REPLACES doc.background when the caller passes one
  // (including explicit null). Project the doc WITHOUT its own background so we
  // control background placement and letterbox fill here.
  const effectiveBackground =
    opts.background !== undefined ? opts.background : doc.background;

  // toKonva sizes the stage to the logical canvas and adds one Konva.Layer per
  // document layer. Re-home those layers onto a stage sized to the output frame.
  const projected = toKonva({ ...doc, background: null });
  const contentLayers = projected.getLayers();

  const stage = new Konva.Stage({
    container: document.createElement("div"),
    width: opts.outWidth,
    height: opts.outHeight,
  });

  if (effectiveBackground != null) {
    const bg = new Konva.Layer({ listening: false });
    bg.add(
      new Konva.Rect({
        x: 0,
        y: 0,
        width: opts.outWidth,
        height: opts.outHeight,
        fill: effectiveBackground,
      }),
    );
    stage.add(bg);
  }

  for (const layer of contentLayers) {
    layer.position({ x: dx, y: dy });
    layer.scale({ x: scale, y: scale });
    layer.moveTo(stage);
  }
  projected.destroy();

  return stage.toBlob({
    pixelRatio: opts.pixelRatio ?? 1,
    mimeType: "image/png",
    width: opts.outWidth,
    height: opts.outHeight,
  }) as Promise<Blob>;
}
