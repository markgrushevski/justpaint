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
 * Host UIs (a Vue view) subscribe with {@link Editor.onChange} and read
 * {@link Editor.getLayers} / {@link Editor.canUndo} after each change — the
 * editor never imports Vue (ARCHITECTURE §3).
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
    this.history.clear();
    this.rerender();
    this.emitChange();
  }

  toPNG(opts: RenderOptions): Promise<Blob> {
    return renderToPNG(this.doc, opts);
  }

  /**
   * Subscribe to editor-state changes (a commit, undo/redo, layer op, or active-
   * layer switch). Returns an unsubscribe function. The callback should re-read
   * {@link getLayers} / {@link getActiveLayerId} / {@link canUndo} / {@link canRedo}.
   */
  onChange(cb: () => void): () => void {
    this.listeners.add(cb);
    return () => {
      this.listeners.delete(cb);
    };
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

  /**
   * Tear down the editor: destroy the Konva stage so it leaves Konva's
   * module-global stage registry and its backing `<canvas>` elements are
   * released (GC alone can't free a stage Konva still references). Call from the
   * host's unmount hook; the instance is unusable afterwards.
   */
  destroy(): void {
    this.listeners.clear();
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

      if (stroke && this.activeLayer()) {
        // Commit routes through history (→ rerender + notify).
        this.commit(addStrokeCommand(this.activeLayerId, stroke));
      } else {
        // Degenerate gesture / no active layer: just discard the preview.
        this.rerender();
      }
    });
  }
}
