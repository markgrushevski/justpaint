/**
 * The editor runtime: owns the canonical {@link Document}, drives a
 * `Konva.Stage` projection, and turns pointer drags into committed strokes via
 * the active {@link Tool}. Every document mutation goes through a command on the
 * {@link History} stack, so undo/redo is exact and the old PNG-snapshot history
 * is gone (ROADMAP Phase 2).
 *
 * The document is authoritative; the stage is a derived projection re-rendered
 * via {@link toKonva} on commit. Tools are pure (no Konva, no state) — the
 * editor feeds them LOGICAL points captured from
 * `stage.getRelativePointerPosition()` (never `pageX`/`offsetLeft`, the old
 * engine's DPR bug — DOCUMENT-FORMAT §2).
 *
 * FIT / ZOOM (ROADMAP Phase 2): the stage is sized to the VIEWPORT (the
 * container) and the document is scaled into it via the stage's own transform
 * (size + scale + position) — never a CSS transform on the `<canvas>`, so
 * `getRelativePointerPosition` keeps returning logical coordinates at any zoom.
 * The editor auto-fits on resize (until the user zooms/pans), zooms toward the
 * cursor on wheel, and pans on a middle-button drag. See {@link ./view}.
 *
 * Host UIs (a Vue view) subscribe with {@link Editor.onChange} and read
 * {@link Editor.getLayers} / {@link Editor.canUndo} / {@link Editor.getZoom}
 * after each change — the editor never imports Vue (ARCHITECTURE §3).
 *
 * BROWSER-ONLY: needs a real DOM container + Konva stage.
 */
import Konva from "konva";
import {
  DEFAULT_BACKGROUND,
  DEFAULT_CANVAS,
  LIMITS,
} from "@justpaint/document";
import type { Document, Layer } from "@justpaint/document";
import { newId } from "./ids";
import {
  addLayerCommand,
  addStrokeCommand,
  History,
  moveLayerCommand,
  removeLayerCommand,
  renameLayerCommand,
  setLayerOpacityCommand,
  setLayerVisibleCommand,
} from "./history";
import type { Command } from "./history";
import { toKonva } from "./konva";
import { renderToPNG } from "./render";
import type { RenderOptions } from "./render";
import { DEFAULT_STYLE } from "./style";
import { penTool } from "./tools/pen";
import type { LayerView, LogicalPoint, Tool, ToolContext, ToolStyle } from "./types";
import { fitView, panBy, zoomAround, ZOOM_STEP, type ViewState } from "./view";

/** Wheel notch zoom factor (gentler than the button step). */
const WHEEL_STEP = 1.1;

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

/** Clamp a name to the document's rune limit, falling back when empty. */
function clampName(name: string, fallback: string): string {
  const runes = [...name.trim()];
  if (runes.length < 1) return fallback;
  return runes.slice(0, LIMITS.maxNameLen).join("");
}

export class Editor {
  private readonly container: HTMLDivElement;
  private doc: Document;
  private activeLayerId: string;
  private activeTool: Tool = penTool;
  private style: ToolStyle = { ...DEFAULT_STYLE };

  private readonly history = new History();
  private readonly listeners = new Set<() => void>();

  private stage: Konva.Stage;
  /** In-progress gesture points, in logical coords; null when not drawing. */
  private gesture: LogicalPoint[] | null = null;
  /**
   * Transient group holding the live, uncommitted stroke — mounted ON the active
   * layer's Konva layer (not an isolated preview layer), so the stroke's
   * composite previews exactly as it will commit: the eraser's destination-out
   * visibly erases while dragging (DECISIONS 2026-07-04).
   */
  private previewGroup: Konva.Group | null = null;

  /** Viewport (container) size in screen px; {0,0} until the first measure. */
  private viewport = { width: 0, height: 0 };
  /** Current zoom + pan applied to the stage. */
  private view: ViewState = { zoom: 1, panX: 0, panY: 0 };
  /** While true, a resize re-fits the document; a manual zoom/pan turns it off. */
  private autoFit = true;
  /** Active middle-button pan drag: the pointer id + the anchor state. */
  private pan: { pointerId: number; startX: number; startY: number; view: ViewState } | null = null;
  private readonly resizeObserver: ResizeObserver;

  constructor(container: HTMLDivElement, doc?: Document) {
    this.container = container;
    this.doc = doc ?? blankDocument();
    const first = this.doc.layers[0];
    this.activeLayerId = first ? first.id : newId();
    this.stage = toKonva(this.doc, container);
    this.bindPointerEvents();
    // Konva only sees pointerup/cancel INSIDE the container; a release outside
    // would leave the gesture/pan stuck. Window-level fallbacks commit/reset.
    window.addEventListener("pointerup", this.onWindowPointerUp);
    window.addEventListener("pointercancel", this.onWindowPointerCancel);
    this.resizeObserver = new ResizeObserver(() => this.measureAndApply());
    this.resizeObserver.observe(container);
    // Measure once now (the container is in the DOM at construction).
    this.measureAndApply();
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
    this.history.clear();
    this.autoFit = true;
    this.rerender();
    this.fitToViewport();
    this.emitChange();
  }

  toPNG(opts: RenderOptions): Promise<Blob> {
    return renderToPNG(this.doc, opts);
  }

  /**
   * Subscribe to editor-state changes (a commit, undo/redo, layer op, active-
   * layer switch, or zoom). Returns an unsubscribe function. The callback should
   * re-read {@link getLayers} / {@link getActiveLayerId} / {@link canUndo} /
   * {@link canRedo} / {@link getZoom}.
   */
  onChange(cb: () => void): () => void {
    this.listeners.add(cb);
    return () => {
      this.listeners.delete(cb);
    };
  }

  // --- view / zoom ----------------------------------------------------------

  /** Current zoom (screen px per logical unit); 1 = 100%. */
  getZoom(): number {
    return this.view.zoom;
  }

  /** Scale-to-fit + center the document in the viewport; re-enables auto-fit. */
  fitToViewport(): void {
    if (this.viewport.width <= 0 || this.viewport.height <= 0) return;
    this.autoFit = true;
    this.view = fitView(this.doc.width, this.doc.height, this.viewport.width, this.viewport.height);
    this.applyView();
    this.emitChange();
  }

  /** Zoom by a multiplicative factor, anchored at a screen point (default: viewport center). */
  zoomBy(factor: number, centerX?: number, centerY?: number): void {
    if (this.viewport.width <= 0 || this.viewport.height <= 0) return;
    const cx = centerX ?? this.viewport.width / 2;
    const cy = centerY ?? this.viewport.height / 2;
    this.autoFit = false;
    this.view = zoomAround(this.view, factor, cx, cy);
    this.applyView();
    this.emitChange();
  }

  zoomIn(): void {
    this.zoomBy(ZOOM_STEP);
  }

  zoomOut(): void {
    this.zoomBy(1 / ZOOM_STEP);
  }

  /**
   * Tear down the editor: stop observing resize, then destroy the Konva stage so
   * it leaves Konva's module-global stage registry and its backing `<canvas>`
   * elements are released. Call from the host's unmount hook; the instance is
   * unusable afterwards.
   */
  destroy(): void {
    this.resizeObserver.disconnect();
    window.removeEventListener("pointerup", this.onWindowPointerUp);
    window.removeEventListener("pointercancel", this.onWindowPointerCancel);
    this.listeners.clear();
    this.stage.destroy();
    this.previewGroup = null;
    this.gesture = null;
    this.pan = null;
  }

  // --- history --------------------------------------------------------------

  canUndo(): boolean {
    return this.history.canUndo;
  }

  canRedo(): boolean {
    return this.history.canRedo;
  }

  undo(): void {
    if (!this.history.undo(this.doc)) return;
    this.reconcileActiveLayer();
    this.afterMutation();
  }

  redo(): void {
    if (!this.history.redo(this.doc)) return;
    this.reconcileActiveLayer();
    this.afterMutation();
  }

  // --- layers ---------------------------------------------------------------

  getLayers(): LayerView[] {
    return this.doc.layers.map((l) => ({
      id: l.id,
      name: l.name,
      visible: l.visible,
      opacity: l.opacity,
      strokeCount: l.strokes.length,
    }));
  }

  getActiveLayerId(): string {
    return this.activeLayerId;
  }

  /** Switch the active layer (where new strokes land). Not undoable — it's editor UI state, not document state. */
  setActiveLayer(id: string): void {
    if (id === this.activeLayerId) return;
    if (!this.doc.layers.some((l) => l.id === id)) return;
    this.activeLayerId = id;
    this.emitChange();
  }

  /** Add a new empty layer on top and make it active. No-op at the layer cap; returns the new id or null. */
  addLayer(name?: string): string | null {
    if (this.doc.layers.length >= LIMITS.maxLayers) return null;
    const fallback = `Layer ${this.doc.layers.length + 1}`;
    const layer: Layer = {
      id: newId(),
      name: name === undefined ? fallback : clampName(name, fallback),
      visible: true,
      opacity: 1,
      strokes: [],
    };
    this.activeLayerId = layer.id;
    this.commit(addLayerCommand(layer, this.doc.layers.length));
    return layer.id;
  }

  /** Remove a layer. No-op on the last remaining layer (documents need ≥1). */
  removeLayer(id: string): void {
    if (this.doc.layers.length <= 1) return;
    const index = this.doc.layers.findIndex((l) => l.id === id);
    if (index === -1) return;
    const wasActive = this.activeLayerId === id;
    this.commit(removeLayerCommand(this.doc, id));
    if (wasActive) {
      // Prefer the layer that slid into this slot (was just above), else the top.
      const next = this.doc.layers[index] ?? this.doc.layers[this.doc.layers.length - 1];
      if (next) this.activeLayerId = next.id;
    }
  }

  /** Move a layer to a new z-index (0 = bottom). Clamped into range. */
  moveLayer(id: string, toIndex: number): void {
    const from = this.doc.layers.findIndex((l) => l.id === id);
    if (from === -1) return;
    const clamped = Math.max(0, Math.min(toIndex, this.doc.layers.length - 1));
    if (clamped === from) return;
    this.commit(moveLayerCommand(this.doc, id, clamped));
  }

  renameLayer(id: string, name: string): void {
    const layer = this.doc.layers.find((l) => l.id === id);
    if (!layer) return;
    const next = clampName(name, layer.name);
    if (next === layer.name) return;
    this.commit(renameLayerCommand(this.doc, id, next));
  }

  setLayerVisible(id: string, visible: boolean): void {
    const layer = this.doc.layers.find((l) => l.id === id);
    if (!layer || layer.visible === visible) return;
    this.commit(setLayerVisibleCommand(this.doc, id, visible));
  }

  setLayerOpacity(id: string, opacity: number): void {
    const layer = this.doc.layers.find((l) => l.id === id);
    if (!layer) return;
    const clamped = Math.max(0, Math.min(1, opacity));
    if (!Number.isFinite(clamped) || clamped === layer.opacity) return;
    this.commit(setLayerOpacityCommand(this.doc, id, clamped));
  }

  // --- internals ------------------------------------------------------------

  private toolContext(): ToolContext {
    return { style: this.style, newId };
  }

  private activeLayer(): Layer | undefined {
    return this.doc.layers.find((l) => l.id === this.activeLayerId);
  }

  /** Execute a command on the history stack, then re-project + notify. */
  private commit(cmd: Command): void {
    this.history.execute(this.doc, cmd);
    this.afterMutation();
  }

  /** Re-project the document and notify subscribers. */
  private afterMutation(): void {
    this.rerender();
    this.emitChange();
  }

  private emitChange(): void {
    for (const cb of this.listeners) cb();
  }

  /** After an undo/redo the active layer may have vanished/returned — keep it valid. */
  private reconcileActiveLayer(): void {
    if (this.doc.layers.some((l) => l.id === this.activeLayerId)) return;
    const first = this.doc.layers[0];
    if (first) this.activeLayerId = first.id;
  }

  /** Re-read the container size; re-fit if auto-fitting, else just re-apply. */
  private measureAndApply(): void {
    const width = this.container.clientWidth;
    const height = this.container.clientHeight;
    if (width <= 0 || height <= 0) return;
    const changed = width !== this.viewport.width || height !== this.viewport.height;
    this.viewport = { width, height };
    if (this.autoFit) {
      this.view = fitView(this.doc.width, this.doc.height, width, height);
    }
    this.applyView();
    if (changed) this.emitChange();
  }

  /**
   * Push the current viewport + view onto the stage: size it to the container,
   * scale + position it so the document sits at `view.zoom`/`view.pan`. This is
   * the ONLY place the stage transform is set — pointer coords stay logical.
   */
  private applyView(): void {
    const w = this.viewport.width || this.doc.width;
    const h = this.viewport.height || this.doc.height;
    this.stage.size({ width: w, height: h });
    this.stage.scale({ x: this.view.zoom, y: this.view.zoom });
    this.stage.position({ x: this.view.panX, y: this.view.panY });
    this.stage.batchDraw();
  }

  /** Tear down the stage and re-project the current document, keeping the view. */
  private rerender(): void {
    this.stage.destroy();
    this.previewGroup = null;
    // Any in-flight gesture/pan belongs to the destroyed stage — its pointer id
    // can never match the new stage, so drop it or it becomes stuck dead state.
    this.gesture = null;
    this.pan = null;
    this.stage = toKonva(this.doc, this.container);
    this.bindPointerEvents();
    this.applyView();
  }

  /** Capture the current pointer position in LOGICAL document coords. */
  private readPoint(evt: Konva.KonvaEventObject<PointerEvent>): LogicalPoint | null {
    const pos = this.stage.getRelativePointerPosition();
    if (!pos) return null;
    return { x: pos.x, y: pos.y, pressure: evt.evt.pressure || 0.5 };
  }

  /**
   * Draw the in-flight stroke onto the ACTIVE layer's Konva layer (topmost via
   * the preview group), so its composite previews exactly as it will commit —
   * an isolated preview layer could never show destination-out erasing the
   * content beneath it. The target layer's clip also bounds the preview.
   */
  private renderPreview(): void {
    if (!this.gesture) return;
    const target = this.activeKonvaLayer();
    if (!target) return;
    const stroke = this.activeTool.buildStroke(this.toolContext(), this.gesture);

    if (!this.previewGroup) {
      this.previewGroup = new Konva.Group({ listening: false });
      target.add(this.previewGroup);
    }
    this.previewGroup.destroyChildren();
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
        for (const node of layer.getChildren()) node.moveTo(this.previewGroup);
      }
      preview.destroy();
    }
    target.batchDraw();
  }

  /** The stage layer projecting the active document layer (offset by the background layer). */
  private activeKonvaLayer(): Konva.Layer | null {
    const idx = this.doc.layers.findIndex((l) => l.id === this.activeLayerId);
    if (idx === -1) return null;
    const offset = this.doc.background != null ? 1 : 0;
    return this.stage.getLayers()[offset + idx] ?? null;
  }

  /** Drop the in-flight preview nodes without a full re-render. */
  private clearPreview(): void {
    if (!this.previewGroup) return;
    const layer = this.previewGroup.getLayer();
    this.previewGroup.destroy();
    this.previewGroup = null;
    layer?.batchDraw();
  }

  /**
   * A gesture may only START inside the document; it may extend past the edge
   * (visually clipped — Figma-frame style, DECISIONS 2026-07-04).
   */
  private insideDocument(pt: LogicalPoint): boolean {
    return pt.x >= 0 && pt.y >= 0 && pt.x <= this.doc.width && pt.y <= this.doc.height;
  }

  /** Commit (or discard) the in-flight gesture; pt is the final point when known. */
  private finishGesture(pt: LogicalPoint | null): void {
    if (!this.gesture) return;
    if (pt) this.gesture.push(pt);
    const stroke = this.activeTool.buildStroke(this.toolContext(), this.gesture);
    this.gesture = null;

    if (stroke && this.activeLayer()) {
      // Commit routes through history (→ rerender + notify); rerender drops the preview.
      this.commit(addStrokeCommand(this.activeLayerId, stroke));
    } else {
      // Degenerate gesture / no active layer: just discard the preview.
      this.clearPreview();
    }
  }

  /** A pointer released OUTSIDE the container still ends the gesture/pan. */
  private readonly onWindowPointerUp = (): void => {
    this.pan = null;
    this.finishGesture(null); // no-op when the stage handler already ran
  };

  /** A cancelled pointer (e.g. touch interrupted) abandons the gesture/pan. */
  private readonly onWindowPointerCancel = (): void => {
    this.pan = null;
    this.gesture = null;
    this.clearPreview();
  };

  private bindPointerEvents(): void {
    this.stage.on("pointerdown", (evt) => {
      // Middle-button drag pans the view instead of drawing.
      if (evt.evt.button === 1) {
        evt.evt.preventDefault();
        const p = this.stage.getPointerPosition();
        if (p) this.pan = { pointerId: evt.evt.pointerId, startX: p.x, startY: p.y, view: { ...this.view } };
        return;
      }
      const pt = this.readPoint(evt);
      // Gestures starting OUTSIDE the document (the letterbox area) are ignored.
      if (!pt || !this.insideDocument(pt)) return;
      this.gesture = [pt];
      this.renderPreview();
    });

    this.stage.on("pointermove", (evt) => {
      if (this.pan && evt.evt.pointerId === this.pan.pointerId) {
        const p = this.stage.getPointerPosition();
        if (p) {
          this.view = panBy(this.pan.view, p.x - this.pan.startX, p.y - this.pan.startY);
          this.applyView();
        }
        return;
      }
      if (!this.gesture) return;
      const pt = this.readPoint(evt);
      if (!pt) return;
      this.gesture.push(pt);
      this.renderPreview();
    });

    this.stage.on("pointerup", (evt) => {
      if (this.pan && evt.evt.pointerId === this.pan.pointerId) {
        this.pan = null;
        return;
      }
      if (!this.gesture) return;
      this.finishGesture(this.readPoint(evt));
    });

    // Wheel zooms toward the cursor.
    this.stage.on("wheel", (evt) => {
      evt.evt.preventDefault();
      const p = this.stage.getPointerPosition();
      if (!p) return;
      const factor = evt.evt.deltaY < 0 ? WHEEL_STEP : 1 / WHEEL_STEP;
      this.autoFit = false;
      this.view = zoomAround(this.view, factor, p.x, p.y);
      this.applyView();
      this.emitChange();
    });
  }
}
