/**
 * Document → raster, the deterministic export (DOCUMENT-FORMAT §10).
 *
 * `renderToStage` builds the framed `Konva.Stage` and is DOM-free (via
 * `stageConfig`), so it is the ONE shared projection used by both the browser
 * (`renderToPNG` → `stage.toBlob`) and the headless Node render worker
 * (`packages/render` → `stage.toDataURL()`, under `konva/canvas-backend`).
 * `renderToPNG` itself stays browser-only (its `toBlob` needs a DOM) and is not
 * unit-tested (no DOM in the Vitest runner).
 */
import Konva from "konva";
import { computeFitTransform } from "@justpaint/document";
import type { Document } from "@justpaint/document";
import { stageConfig, toKonva } from "./konva";

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
export function renderToStage(doc: Document, opts: RenderOptions): Konva.Stage {
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

  const stage = new Konva.Stage(stageConfig(undefined, opts.outWidth, opts.outHeight));

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

  return stage;
}

/**
 * Render `doc` to a PNG Blob (browser). Thin wrapper over {@link renderToStage} +
 * `stage.toBlob`. The Node render worker calls `renderToStage` directly and
 * serializes via `stage.toDataURL()` (node-canvas), sharing the exact projection
 * + fit path so the editor preview and the judged raster agree.
 */
export async function renderToPNG(doc: Document, opts: RenderOptions): Promise<Blob> {
  const stage = renderToStage(doc, opts);
  try {
    return (await stage.toBlob({
      pixelRatio: opts.pixelRatio ?? 1,
      mimeType: "image/png",
      width: opts.outWidth,
      height: opts.outHeight,
    })) as Blob;
  } finally {
    // toBlob does NOT destroy the stage; without this, every export leaks the
    // output stage (+ its <canvas>) into Konva's module-global registry. The Node
    // worker owns its own renderToStage stage, so only this browser wrapper frees.
    stage.destroy();
  }
}
