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
 * cursor on wheel, and pans on a middle-button drag with any tool — or on a
 * PRIMARY drag (mouse or single-finger touch) when the hand tool is active.
 * Both routes share one pan path. See {@link ./view}.
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
import type { Document, Layer, Op, Stroke } from "@justpaint/document";
import { newId } from "./ids";
import {
  addLayerCommand,
  addStrokeCommand,
  compositeCommand,
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
import type { LayerView, LogicalPoint, StrokeTool, Tool, ToolContext, ToolStyle } from "./types";
import { fitView, panBy, zoomAround, ZOOM_STEP, type ViewState } from "./view";

/** Wheel notch zoom factor (gentler than the button step). */
const WHEEL_STEP = 1.1;

/**
 * "Sheet of paper on a desk": the backdrop rect's drop shadow, in SCREEN px.
 * Konva shadows live in stage coordinates and would zoom with the drawing, so
 * blur/offset are counter-scaled by 1/zoom in {@link Editor.syncBackdropScreenSpace}.
 */
const BACKDROP_SHADOW = {
  color: "black",
  opacity: 0.22,
  blur: 12,
  offset: { x: 0, y: 2 },
} as const;

/** The document's hairline edge (1 screen px via `strokeScaleEnabled: false`). */
const BACKDROP_BORDER = { color: "rgb(0 0 0 / 25%)", width: 1 } as const;

/**
 * The AI-assist ghost overlay (ASSIST.md §5): a proposed batch renders on a
 * non-listening top layer at reduced opacity with a dashed accent frame — a
 * proposal, NOT yet in the document or history. The frame reuses the host-bridged
 * cursor color (its brand accent) when present, else this neutral fallback; it is
 * view chrome only and can never leak into an export (the layer lives solely on
 * the interactive stage — {@link Editor.toPNG} builds a fresh stage from the doc).
 */
const GHOST_OPACITY = 0.55;
const GHOST_ACCENT = "#4f7cff";
/** Dashed frame around the proposal region, in screen px (hairline at any zoom). */
const GHOST_FRAME_DASH = [6, 4] as const;

/**
 * A VIEW-ONLY backdrop painted BEHIND the document (the host uses it for
 * theme "paper" white/black, or a transparency checkerboard). Pure presentation
 * state: it is NOT part of the {@link Document}, never serialized, and never
 * exported — {@link Editor.toPNG} / `renderToPNG` (src/render.ts) build a FRESH
 * stage from the document alone, on which this layer does not exist.
 */
export type CanvasBackdrop =
  | { type: "color"; color: string }
  | { type: "pattern"; image: CanvasImageSource };

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

  /**
   * The brush-size cursor ring (UI chrome, never document data). The color is a
   * RESOLVED CSS color string passed in by the host (canvas cannot read CSS
   * custom properties, and the editor stays design-token-agnostic — the host
   * bridges its theme, e.g. oriUI's `--ori-color-primary`, and re-calls
   * {@link setCursorColor} on theme flips). `null` disables the ring.
   */
  private cursorColor: string | null = null;
  /** Last hover position in logical coords; null = hidden (touch, off-canvas). */
  private cursor: { x: number; y: number } | null = null;
  /** Non-listening top layer holding the ring; rebuilt with the stage. */
  private cursorLayer: Konva.Layer | null = null;
  private cursorRing: Konva.Circle | null = null;

  /**
   * The view-only backdrop (see {@link CanvasBackdrop}); null = none. Like the
   * cursor ring it is stage chrome: remounted on every rerender, never part of
   * the document.
   */
  private backdrop: CanvasBackdrop | null = null;
  /** Non-listening BOTTOM-MOST layer holding the backdrop rect; rebuilt with the stage. */
  private backdropLayer: Konva.Layer | null = null;
  private backdropRect: Konva.Rect | null = null;

  /**
   * A pending AI-assist proposal (see {@link previewOps}). `ghostOps` is the
   * batch data — kept so the overlay can be REMOUNTED after a `rerender` rebuilds
   * the stage (mirroring the cursor overlay); `ghostLayer` is its non-listening
   * top Konva layer. Neither is in the document or history until {@link acceptOps}.
   */
  private ghostOps: Op[] | null = null;
  private ghostLayer: Konva.Layer | null = null;

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
    // Escape mid-drag abandons an in-flight pan (keyboard-flavored pointercancel).
    window.addEventListener("keydown", this.onWindowKeyDown);
    // The stage only sees moves INSIDE the container — leaving it must hide the ring.
    container.addEventListener("pointerleave", this.onContainerPointerLeave);
    this.resizeObserver = new ResizeObserver(() => this.measureAndApply());
    this.resizeObserver.observe(container);
    // Measure once now (the container is in the DOM at construction).
    this.measureAndApply();
  }

  // --- public API -----------------------------------------------------------

  setTool(tool: Tool): void {
    this.activeTool = tool;
    // Switching to the hand mid-drag: a pan tool cannot finish a stroke
    // gesture — abandon it (never half-commit through the wrong tool).
    if (tool.kind === "pan" && this.gesture) {
      this.gesture = null;
      this.clearPreview();
    }
    this.syncCursorRing(); // the brush ring hides while the hand is active
    this.syncContainerCursor();
  }

  setStyle(patch: Partial<ToolStyle>): void {
    this.style = { ...this.style, ...patch };
    this.syncCursorRing(); // the ring's diameter tracks strokeWidth
  }

  /**
   * Enable/re-color the brush-size cursor ring — a circle following the pointer,
   * `strokeWidth` in diameter in LOGICAL units (so it scales with zoom), shown
   * for mouse/pen and hidden on touch or off-canvas. Pass a resolved CSS color
   * (e.g. a design token resolved by the host); `null` turns the ring off.
   */
  setCursorColor(color: string | null): void {
    this.cursorColor = color;
    if (color == null) {
      this.cursorLayer?.destroy();
      this.cursorLayer = null;
      this.cursorRing = null;
      return;
    }
    if (!this.cursorLayer) this.mountCursorOverlay();
    this.syncCursorRing();
  }

  /**
   * Set (or clear, with `null`) the view-only canvas backdrop painted BEHIND
   * the document — below even a non-null `doc.background`. Presentation state
   * only: {@link getDocument} is unaffected, and exports cannot pick it up
   * (see {@link mountBackdrop}). A pattern tiles in ~SCREEN space: its scale is
   * re-compensated on every zoom change so a checkerboard never zooms with the
   * drawing. Either flavor reads as a sheet of paper on a desk: the rect
   * carries a drop shadow + a 1px border, both ~screen-space too (see
   * {@link syncBackdropScreenSpace}) — view chrome only, never exported.
   */
  setCanvasBackdrop(backdrop: CanvasBackdrop | null): void {
    this.backdrop = backdrop;
    // Recreate from scratch on every change: switching color <-> pattern on one
    // rect would have to un-set the other fill's attrs; a fresh rect can't leak.
    this.backdropLayer?.destroy();
    this.backdropLayer = null;
    this.backdropRect = null;
    if (backdrop != null) this.mountBackdrop();
  }

  getDocument(): Document {
    return this.doc;
  }

  loadDocument(doc: Document): void {
    this.doc = doc;
    const first = doc.layers[0];
    this.activeLayerId = first ? first.id : newId();
    this.history.clear();
    // Drop any pending AI-assist proposal — it referenced the OLD document; nulling
    // ghostOps before rerender stops it from remounting against the new doc.
    this.clearGhost();
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

  /**
   * Map a viewport/client point (e.g. `PointerEvent.clientX`/`clientY`) to
   * LOGICAL document coordinates through the current stage transform — the
   * inverse of the same transform `getRelativePointerPosition` reads for
   * drawing, so a host coordinate readout matches where a stroke would land
   * exactly, at any zoom/pan.
   *
   * Returns `null` when the point lies outside the stage container. Inside it,
   * coordinates are returned RAW (unrounded) and may fall outside the document
   * rect (the letterbox maps to negative / >size values) — rounding, clamping,
   * and formatting are presentation and belong to the host. The editor adds NO
   * listeners for this: subscribe to `pointermove` yourself and call it.
   */
  toDocumentCoords(clientX: number, clientY: number): { x: number; y: number } | null {
    // Konva's own pointer math measures stage.content; headless (tests, no
    // DOM build) it does not exist — the container is its parent and the stage
    // is sized to it, so its rect is the same frame.
    const el = (this.stage.content as HTMLDivElement | undefined) ?? this.container;
    const rect = el.getBoundingClientRect();
    const sx = clientX - rect.left;
    const sy = clientY - rect.top;
    if (sx < 0 || sy < 0 || sx > rect.width || sy > rect.height) return null;
    // Invert the stage transform (applyView is its only writer): screen → logical.
    const p = this.stage.getAbsoluteTransform().copy().invert().point({ x: sx, y: sy });
    return { x: p.x, y: p.y };
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
    window.removeEventListener("keydown", this.onWindowKeyDown);
    this.container.removeEventListener("pointerleave", this.onContainerPointerLeave);
    // Give the cursor back to the host iff we own it (hand active / mid-pan).
    const cursor = this.container.style.cursor;
    if (cursor === "grab" || cursor === "grabbing") this.container.style.cursor = "";
    this.listeners.clear();
    this.stage.destroy();
    this.previewGroup = null;
    this.cursorLayer = null;
    this.cursorRing = null;
    this.backdropLayer = null;
    this.backdropRect = null;
    this.ghostLayer = null;
    this.ghostOps = null;
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

  // --- ai assist (ghost preview; see ASSIST.md §5) --------------------------

  /**
   * Render an AI-assist proposal as a GHOST overlay — a top, non-listening Konva
   * layer at reduced opacity with a dashed accent frame, clipped to the doc rect,
   * projecting the batch's strokes. The proposal is NOT in the document and NOT in
   * history until {@link acceptOps}. Re-previewing replaces the prior proposal.
   *
   * The op → command mapping and this overlay live entirely inside the editor: the
   * app passes validated {@link Op}s and never touches Konva or a command.
   */
  previewOps(ops: Op[]): void {
    this.clearGhost();
    this.ghostOps = ops;
    this.mountGhostOverlay();
  }

  /**
   * Commit the previewed proposal as ONE composite command — a single undo entry,
   * so one Ctrl+Z removes the whole batch — through the normal commit path, then
   * tear down the ghost. No-op when nothing is previewed. Ops apply in array order
   * with a batch-local id map: an `add_layer` mints a fresh layer id and an
   * `add_stroke` resolves its `layerId` against that map (an earlier batch layer)
   * or an existing document layer. Strokes arrive pre-built + server-validated —
   * they map straight onto commands, never re-run through a tool.
   */
  acceptOps(): void {
    const ops = this.ghostOps;
    if (ops == null) return;
    // An empty batch is a VALID proposal (nothing to add) but committing an empty
    // composite would still push a no-op entry onto the undo stack — a phantom
    // Ctrl+Z that does nothing. Tear down the ghost and bail before committing.
    if (ops.length === 0) {
      this.clearGhost();
      return;
    }
    const commands: Command[] = [];
    /** Batch-local `add_layer.id` → the fresh real id assigned here. */
    const idMap = new Map<string, string>();
    // addLayerCommand clamps its index at APPLY time against doc.layers.length, so
    // several add_layer ops need a RUNNING top index — a stale length would clamp
    // every new layer to the same slot and collide their z-order.
    let topIndex = this.doc.layers.length;
    for (const op of ops) {
      if (op.kind === "add_layer") {
        const realId = newId();
        idMap.set(op.id, realId);
        const layer: Layer = {
          id: realId,
          name: op.name,
          visible: true,
          opacity: 1,
          strokes: [],
        };
        commands.push(addLayerCommand(layer, topIndex));
        topIndex += 1;
      } else {
        const resolvedId = idMap.get(op.layerId) ?? op.layerId;
        commands.push(addStrokeCommand(resolvedId, op.stroke));
      }
    }
    // Tear the ghost down BEFORE committing: commit → rerender, whose ghost-remount
    // guard reads ghostOps — leaving it set would re-draw the now-committed batch.
    this.clearGhost();
    this.commit(compositeCommand(commands, "AI assist"));
  }

  /** Discard the previewed proposal — nothing enters the document or history. */
  rejectOps(): void {
    if (this.ghostOps == null) return;
    this.clearGhost();
    this.stage.batchDraw();
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
    // The ONE zoom hook: every zoom/pan/fit funnels through here, so the
    // backdrop's screen-space compensation (pattern tiles, shadow) can't
    // drift out of sync.
    this.syncBackdropScreenSpace();
    this.stage.batchDraw();
  }

  /** Tear down the stage and re-project the current document, keeping the view. */
  private rerender(): void {
    this.stage.destroy();
    this.previewGroup = null;
    this.cursorLayer = null;
    this.cursorRing = null;
    this.backdropLayer = null;
    this.backdropRect = null;
    // The ghost layer died with the stage; keep ghostOps (the proposal data) so it
    // can remount below, but clear the stale layer handle.
    this.ghostLayer = null;
    // Any in-flight gesture/pan belongs to the destroyed stage — its pointer id
    // can never match the new stage, so drop it or it becomes stuck dead state.
    this.gesture = null;
    this.pan = null;
    this.stage = toKonva(this.doc, this.container);
    this.bindPointerEvents();
    // The backdrop died with the old stage; remount it against the CURRENT doc
    // (loadDocument may have changed the canvas size) before any layer math runs.
    if (this.backdrop != null) this.mountBackdrop();
    // The overlay died with the old stage; remount it (the ring reappears at the
    // last hover point instead of blinking out after every commit).
    if (this.cursorColor != null) {
      this.mountCursorOverlay();
      this.syncCursorRing();
    }
    // A pending AI-assist proposal (previewOps) also died with the stage; remount
    // it the same way or it silently vanishes on the next commit/undo/redo.
    if (this.ghostOps != null) this.mountGhostOverlay();
    this.applyView();
    // The pan was force-dropped above — don't leave a stale "grabbing" cursor.
    this.syncContainerCursor();
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
    const tool = this.strokeTool();
    if (!this.gesture || !tool) return;
    const target = this.activeKonvaLayer();
    if (!target) return;
    const stroke = tool.buildStroke(this.toolContext(), this.gesture);

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

  /**
   * The stage layer projecting the active document layer. Stage z-order is
   * `[backdrop?] [background?] [doc layers...] [cursor overlay?]`, so the
   * document-index → stage-index mapping is offset by BOTH optional bottom
   * layers. This is the single place that mapping lives — the preview group
   * (eraser preview included) mounts through here.
   */
  private activeKonvaLayer(): Konva.Layer | null {
    const idx = this.doc.layers.findIndex((l) => l.id === this.activeLayerId);
    if (idx === -1) return null;
    const offset =
      (this.backdropLayer ? 1 : 0) + (this.doc.background != null ? 1 : 0);
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

  /** The active tool when it draws strokes; null while the hand (pan) tool is up. */
  private strokeTool(): StrokeTool | null {
    return this.activeTool.kind === "stroke" ? this.activeTool : null;
  }

  /** Commit (or discard) the in-flight gesture; pt is the final point when known. */
  private finishGesture(pt: LogicalPoint | null): void {
    if (!this.gesture) return;
    const tool = this.strokeTool();
    if (!tool) {
      // Defensive: setTool already drops the gesture on a switch to a pan tool.
      this.gesture = null;
      this.clearPreview();
      return;
    }
    if (pt) this.gesture.push(pt);
    const stroke = tool.buildStroke(this.toolContext(), this.gesture);
    this.gesture = null;

    if (stroke && this.activeLayer()) {
      // Commit routes through history (→ rerender + notify); rerender drops the preview.
      this.commit(addStrokeCommand(this.activeLayerId, stroke));
    } else {
      // Degenerate gesture / no active layer: just discard the preview.
      this.clearPreview();
    }
  }

  // --- canvas backdrop (see setCanvasBackdrop) -------------------------------

  /**
   * Create the backdrop layer + rect on the CURRENT stage and sink it to the
   * bottom, below even the document background layer. Reads `this.backdrop` and
   * the CURRENT doc dimensions, so {@link rerender} / {@link loadDocument} can
   * simply remount after the stage is rebuilt.
   *
   * EXPORT SAFETY: this layer lives ONLY on the interactive stage. `toPNG` /
   * `renderToPNG` (src/render.ts) project a FRESH stage from the document via
   * `renderToStage`, and the backdrop is not document state — it can never leak
   * into an exported or judged raster.
   */
  private mountBackdrop(): void {
    if (!this.backdrop) return;
    const { width, height } = this.doc;
    // NOT clipped, unlike every projected layer (konva.ts): the drop shadow and
    // the outer half of the centered border render OUTSIDE the doc rect, and a
    // doc-rect clip would swallow both. Safe to skip — the rect itself covers
    // exactly the doc bounds and nothing else ever mounts on this layer.
    const layer = new Konva.Layer({ listening: false });
    const rect = new Konva.Rect({
      x: 0,
      y: 0,
      width,
      height,
      listening: false,
      // "Paper on a desk": drop shadow + 1px document edge. Shadow blur/offset
      // are zoom-compensated in syncBackdropScreenSpace; the border stays a
      // 1-screen-px hairline via strokeScaleEnabled (the cursor-ring trick).
      shadowColor: BACKDROP_SHADOW.color,
      shadowOpacity: BACKDROP_SHADOW.opacity,
      shadowForStrokeEnabled: false,
      stroke: BACKDROP_BORDER.color,
      strokeWidth: BACKDROP_BORDER.width,
      strokeScaleEnabled: false,
    });
    layer.add(rect);
    this.backdropLayer = layer;
    this.backdropRect = rect;
    if (this.backdrop.type === "color") {
      rect.fill(this.backdrop.color);
    } else {
      // Konva's setter is typed to the element flavors, but it only feeds
      // canvas `createPattern()`, which accepts any CanvasImageSource.
      rect.fillPatternImage(this.backdrop.image as HTMLImageElement);
      rect.fillPatternRepeat("repeat");
    }
    this.syncBackdropScreenSpace();
    this.stage.add(layer);
    layer.moveToBottom();
    layer.batchDraw();
  }

  /**
   * Keep the backdrop's screen-space attrs ~SCREEN-SPACE: the stage transform
   * scales everything by `view.zoom`, so anything that must read constant on
   * screen is counter-scaled by `1/zoom` — pattern tiles (a checkerboard
   * shouldn't zoom with the drawing) and the drop shadow's blur/offset (the
   * paper sits the same height off the desk at any zoom). The border needs no
   * entry here: `strokeScaleEnabled: false` already pins it to 1 screen px.
   * Called from {@link applyView} — the one place the stage transform is set —
   * and on backdrop creation.
   */
  private syncBackdropScreenSpace(): void {
    if (!this.backdropRect) return;
    const s = 1 / this.view.zoom; // zoom is clamped to [MIN_ZOOM, MAX_ZOOM], never 0
    this.backdropRect.shadowBlur(BACKDROP_SHADOW.blur * s);
    this.backdropRect.shadowOffset({
      x: BACKDROP_SHADOW.offset.x * s,
      y: BACKDROP_SHADOW.offset.y * s,
    });
    if (this.backdrop?.type === "pattern") {
      this.backdropRect.fillPatternScale({ x: s, y: s });
    }
  }

  // --- ai assist ghost overlay (see previewOps) ------------------------------

  /**
   * Build the ghost layer on the CURRENT stage from `this.ghostOps` and float it
   * on top. Reads `this.ghostOps` + the CURRENT doc, so {@link rerender} simply
   * remounts after the stage is rebuilt (mirroring the cursor overlay). Only
   * `add_stroke` ops paint — `add_layer` ops make empty layers (nothing to draw).
   * The strokes project through a throwaway one-layer doc, the same idiom as
   * {@link renderPreview}; the layer is clipped to the doc rect exactly like a
   * committed layer ({@link toLayer}), so a ghost stroke past the edge previews
   * identically to how it will commit.
   */
  private mountGhostOverlay(): void {
    // An empty batch (a valid proposal — nothing to add) has nothing to preview;
    // skip mounting so we don't float a bare dashed frame over the whole canvas.
    // This guard covers both call sites: previewOps() and the rerender() remount.
    if (this.ghostOps == null || this.ghostOps.length === 0) return;
    const { width, height } = this.doc;
    const layer = new Konva.Layer({
      listening: false,
      opacity: GHOST_OPACITY,
      clip: { x: 0, y: 0, width, height },
    });
    const strokes: Stroke[] = [];
    for (const op of this.ghostOps) {
      if (op.kind === "add_stroke") strokes.push(op.stroke);
    }
    if (strokes.length > 0) {
      const projected = toKonva({
        ...this.doc,
        background: null,
        layers: [{ id: "ghost", name: "ghost", visible: true, opacity: 1, strokes }],
      });
      for (const projectedLayer of projected.getLayers()) {
        for (const node of projectedLayer.getChildren()) node.moveTo(layer);
      }
      projected.destroy();
    }
    // A dashed accent frame marks the region as a PROPOSAL (ASSIST.md §5). Hairline
    // at any zoom via strokeScaleEnabled; color reuses the host-bridged accent.
    layer.add(
      new Konva.Rect({
        x: 0,
        y: 0,
        width,
        height,
        listening: false,
        stroke: this.cursorColor ?? GHOST_ACCENT,
        strokeWidth: 1,
        strokeScaleEnabled: false,
        dash: [...GHOST_FRAME_DASH],
      }),
    );
    this.ghostLayer = layer;
    this.stage.add(layer);
    layer.moveToTop();
    layer.batchDraw();
  }

  /** Tear down the ghost overlay + drop the pending proposal (no doc/history touch). */
  private clearGhost(): void {
    this.ghostLayer?.destroy();
    this.ghostLayer = null;
    this.ghostOps = null;
  }

  // --- cursor ring (see setCursorColor) --------------------------------------

  /** Create the non-listening overlay layer + ring on the CURRENT stage, topmost. */
  private mountCursorOverlay(): void {
    this.cursorLayer = new Konva.Layer({ listening: false });
    this.cursorRing = new Konva.Circle({
      listening: false,
      visible: false,
      strokeWidth: 1,
      // A hairline at every zoom: the RADIUS is logical (scales with the stage
      // transform = brush size in canvas units), the outline stays 1 screen px.
      strokeScaleEnabled: false,
    });
    this.cursorLayer.add(this.cursorRing);
    this.stage.add(this.cursorLayer);
  }

  /** Push color/diameter/position/visibility onto the ring and repaint its layer. */
  private syncCursorRing(): void {
    if (!this.cursorRing || !this.cursorLayer || this.cursorColor == null) return;
    // Hidden while panning AND while the hand tool is armed — the hand never
    // draws, so a brush-size ring would lie (the grab cursor takes over).
    const visible = this.cursor != null && this.pan == null && this.activeTool.kind !== "pan";
    this.cursorRing.visible(visible);
    if (visible && this.cursor) {
      this.cursorRing.position(this.cursor);
      this.cursorRing.radius(Math.max(this.style.strokeWidth / 2, 0.5));
      this.cursorRing.stroke(this.cursorColor);
    }
    this.cursorLayer.batchDraw();
  }

  /** Track the hover point in logical coords; touch never shows the ring. */
  private trackCursor(evt: Konva.KonvaEventObject<PointerEvent>): void {
    if (!this.cursorRing) return;
    // Konva maps BOTH DOM pointermove and touchmove onto its "pointermove": a
    // touch drag can arrive as a PointerEvent (pointerType "touch") or as a raw
    // TouchEvent (no pointerType at all) — mirror Konva's own touch check.
    const isTouch = evt.evt.type.startsWith("touch") || evt.evt.pointerType === "touch";
    if (isTouch) {
      this.cursor = null;
    } else {
      const pos = this.stage.getRelativePointerPosition();
      this.cursor = pos ? { x: pos.x, y: pos.y } : null;
    }
    this.syncCursorRing();
  }

  private readonly onContainerPointerLeave = (): void => {
    this.cursor = null;
    this.syncCursorRing();
  };

  /** A pointer released OUTSIDE the container still ends the gesture/pan. */
  private readonly onWindowPointerUp = (): void => {
    this.endPan(); // no-op without a pan
    this.finishGesture(null); // no-op when the stage handler already ran
  };

  /** A cancelled pointer (e.g. touch interrupted) abandons the gesture/pan. */
  private readonly onWindowPointerCancel = (): void => {
    this.endPan();
    this.gesture = null;
    this.clearPreview();
  };

  /** Escape mid-drag ends an in-flight pan cleanly (the pointercancel hardening, on a key). */
  private readonly onWindowKeyDown = (e: KeyboardEvent): void => {
    if (e.key === "Escape") this.endPan();
  };

  // --- pan (middle-button with any tool; primary pointer with the hand tool) --

  /**
   * Anchor a view-pan drag at the CURRENT pointer position (screen coords —
   * deliberately NOT `readPoint`/`insideDocument`: a pan may grab the letterbox
   * outside the document rect). One shared path: the hand tool's primary drag
   * and the always-available middle-button drag both come through here, so the
   * clamping/transform behavior can never fork. Ignored while another pan is
   * live (a second touch must not re-anchor the view mid-drag).
   */
  private beginPan(pointerId: number): void {
    if (this.pan) return;
    const p = this.stage.getPointerPosition();
    if (!p) return;
    this.pan = { pointerId, startX: p.x, startY: p.y, view: { ...this.view } };
    this.syncCursorRing(); // the brush ring hides for the duration of the pan
    this.syncContainerCursor(); // grab → grabbing
  }

  /** Ends a pan from ANY exit (pointerup, window fallback, cancel, Escape); no-op without one. */
  private endPan(): void {
    if (!this.pan) return;
    this.pan = null;
    this.syncCursorRing();
    this.syncContainerCursor();
  }

  /**
   * Reflect pan/hand state on the CONTAINER's cursor so the host needs no
   * wiring: `grabbing` during any pan, `grab` while the hand tool is armed.
   * Only a grab/grabbing value we set ourselves is ever cleared — a host that
   * styles the container cursor for other tools keeps full ownership of it.
   */
  private syncContainerCursor(): void {
    const style = this.container.style;
    if (this.pan) {
      style.cursor = "grabbing";
    } else if (this.activeTool.kind === "pan") {
      style.cursor = "grab";
    } else if (style.cursor === "grab" || style.cursor === "grabbing") {
      style.cursor = "";
    }
  }

  private bindPointerEvents(): void {
    this.stage.on("pointerdown", (evt) => {
      // PAN routes BEFORE the stroke-gesture path: the hand tool grabs with ANY
      // pointer (primary mouse drag, single-finger touch — Konva may hand us a
      // raw TouchEvent with no `button` at all), and a middle-button drag pans
      // with every tool. No insideDocument gate here: the letterbox is grabbable.
      if (this.activeTool.kind === "pan" || evt.evt.button === 1) {
        evt.evt.preventDefault();
        this.beginPan(evt.evt.pointerId);
        return;
      }
      const pt = this.readPoint(evt);
      // Gestures starting OUTSIDE the document (the letterbox area) are ignored.
      if (!pt || !this.insideDocument(pt)) return;
      this.gesture = [pt];
      this.renderPreview();
    });

    this.stage.on("pointermove", (evt) => {
      this.trackCursor(evt); // ring follows every non-touch move (pan/hand hides it)
      if (this.pan && evt.evt.pointerId === this.pan.pointerId) {
        const p = this.stage.getPointerPosition();
        if (p) {
          // A real pan is a manual view change: stop auto-fit from undoing it
          // on the next resize (same contract as zoomBy/wheel).
          this.autoFit = false;
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
        this.endPan();
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
