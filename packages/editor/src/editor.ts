/**
 * The editor runtime: owns the canonical {@link Document}, drives a
 * `Konva.Stage` projection, and turns pointer drags into committed strokes via
 * the active {@link Tool}.
 *
 * The document is authoritative; the stage is a derived projection re-rendered
 * via {@link toKonva} on commit. Tools are pure (no Konva, no state) — the
 * editor feeds them LOGICAL points captured from
 * `stage.getRelativePointerPosition()` (never `pageX`/`offsetLeft`, the old
 * engine's DPR bug — DOCUMENT-FORMAT §2).
 *
 * BROWSER-ONLY: needs a real DOM container + Konva stage.
 */
import Konva from "konva";
import {
  DEFAULT_BACKGROUND,
  DEFAULT_CANVAS,
} from "@justpaint/document";
import type { Document, Layer } from "@justpaint/document";
import { newId } from "./ids";
import { toKonva } from "./konva";
import { renderToPNG } from "./render";
import type { RenderOptions } from "./render";
import { DEFAULT_STYLE } from "./style";
import { penTool } from "./tools/pen";
import type { LogicalPoint, Tool, ToolContext, ToolStyle } from "./types";

/** A blank single-layer document on the default canvas + background. */
function blankDocument(): Document {
  return {
    version: 1,
    width: DEFAULT_CANVAS.width,
    height: DEFAULT_CANVAS.height,
    background: DEFAULT_BACKGROUND,
    layers: [
      { id: newId(), name: "Layer 1", visible: true, opacity: 1, strokes: [] },
    ],
  };
}

export class Editor {
  private readonly container: HTMLDivElement;
  private doc: Document;
  private activeLayerId: string;
  private activeTool: Tool = penTool;
  private style: ToolStyle = { ...DEFAULT_STYLE };

  private stage: Konva.Stage;
  /** In-progress gesture points, in logical coords; null when not drawing. */
  private gesture: LogicalPoint[] | null = null;
  /** Transient preview layer for the live, uncommitted stroke. */
  private previewLayer: Konva.Layer | null = null;

  constructor(container: HTMLDivElement, doc?: Document) {
    this.container = container;
    this.doc = doc ?? blankDocument();
    const first = this.doc.layers[0];
    this.activeLayerId = first ? first.id : newId();
    this.stage = toKonva(this.doc, container);
    this.bindPointerEvents();
  }

  // --- public API -----------------------------------------------------------

  setTool(tool: Tool): void {
    this.activeTool = tool;
  }

  setStyle(patch: Partial<ToolStyle>): void {
    this.style = { ...this.style, ...patch };
  }

  getDocument(): Document {
    return this.doc;
  }

  loadDocument(doc: Document): void {
    this.doc = doc;
    const first = doc.layers[0];
    this.activeLayerId = first ? first.id : newId();
    this.rerender();
  }

  toPNG(opts: RenderOptions): Promise<Blob> {
    return renderToPNG(this.doc, opts);
  }

  /**
   * Tear down the editor: destroy the Konva stage so it leaves Konva's
   * module-global stage registry and its backing `<canvas>` elements are
   * released (GC alone can't free a stage Konva still references). Call from the
   * host's unmount hook; the instance is unusable afterwards.
   */
  destroy(): void {
    this.stage.destroy();
    this.previewLayer = null;
    this.gesture = null;
  }

  // --- internals ------------------------------------------------------------

  private toolContext(): ToolContext {
    return { style: this.style, newId };
  }

  private activeLayer(): Layer | undefined {
    return this.doc.layers.find((l) => l.id === this.activeLayerId);
  }

  /** Tear down the stage and re-project the current document. */
  private rerender(): void {
    this.stage.destroy();
    this.previewLayer = null;
    this.stage = toKonva(this.doc, this.container);
    this.bindPointerEvents();
  }

  /** Capture the current pointer position in LOGICAL document coords. */
  private readPoint(evt: Konva.KonvaEventObject<PointerEvent>): LogicalPoint | null {
    const pos = this.stage.getRelativePointerPosition();
    if (!pos) return null;
    return { x: pos.x, y: pos.y, pressure: evt.evt.pressure || 0.5 };
  }

  /** Draw the active tool's in-progress stroke into a transient preview layer. */
  private renderPreview(): void {
    if (!this.gesture) return;
    const stroke = this.activeTool.buildStroke(this.toolContext(), this.gesture);

    if (!this.previewLayer) {
      this.previewLayer = new Konva.Layer({ listening: false });
      this.stage.add(this.previewLayer);
    }
    this.previewLayer.destroyChildren();
    if (stroke) {
      // Project the single preview stroke through a throwaway one-layer doc.
      const preview = toKonva({
        ...this.doc,
        background: null,
        layers: [
          { id: "preview", name: "preview", visible: true, opacity: 1, strokes: [stroke] },
        ],
      });
      for (const layer of preview.getLayers()) {
        for (const node of layer.getChildren()) node.moveTo(this.previewLayer);
      }
      preview.destroy();
    }
    this.previewLayer.draw();
  }

  private bindPointerEvents(): void {
    this.stage.on("pointerdown", (evt) => {
      const pt = this.readPoint(evt);
      if (!pt) return;
      this.gesture = [pt];
      this.renderPreview();
    });

    this.stage.on("pointermove", (evt) => {
      if (!this.gesture) return;
      const pt = this.readPoint(evt);
      if (!pt) return;
      this.gesture.push(pt);
      this.renderPreview();
    });

    this.stage.on("pointerup", (evt) => {
      if (!this.gesture) return;
      const pt = this.readPoint(evt);
      if (pt) this.gesture.push(pt);
      const stroke = this.activeTool.buildStroke(this.toolContext(), this.gesture);
      this.gesture = null;

      if (stroke) {
        const layer = this.activeLayer();
        if (layer) layer.strokes.push(stroke);
      }
      this.rerender();
    });
  }
}
