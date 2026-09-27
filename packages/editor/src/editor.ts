/**
 * The editor runtime: owns the canonical {@link Document}, projects it onto a
 * `Konva.Stage` and turns pointer drags into strokes through the active {@link Tool}.
 * Every mutation is a command on the {@link History} stack.
 *
 * Tools get LOGICAL points from `stage.getRelativePointerPosition()`, never
 * `pageX`/`offsetLeft` (a DPR trap, DOCUMENT-FORMAT.md §2). Browser-only, and never
 * imports a host framework (ARCHITECTURE.md §3).
 */
import Konva from 'konva'
import { DEFAULT_BACKGROUND, DEFAULT_CANVAS, LIMITS } from './document'
import type { Document, Layer, Op, Stroke } from './document'
import { newId } from './ids'
import {
    addLayerCommand,
    addStrokeCommand,
    compositeCommand,
    History,
    moveLayerCommand,
    removeLayerCommand,
    renameLayerCommand,
    setLayerOpacityCommand,
    setLayerVisibleCommand
} from './history'
import type { Command } from './history'
import { toKonva } from './konva'
import { renderToPNG } from './render'
import type { RenderOptions } from './render'
import { DEFAULT_STYLE } from './style'
import { penTool } from './tools/pen'
import type { LayerView, LogicalPoint, StrokeTool, Tool, ToolContext, ToolStyle } from './types'
import { fitView, panBy, zoomAround, ZOOM_STEP, type ViewState } from './view'

/** Wheel notch zoom factor (gentler than the button step). */
const WHEEL_STEP = 1.1

/** The backdrop's drop shadow in screen px; counter-scaled by 1/zoom, as Konva shadows zoom. */
const BACKDROP_SHADOW = {
    color: 'black',
    opacity: 0.22,
    blur: 12,
    offset: { x: 0, y: 2 }
} as const

/** The document's hairline edge, 1 screen px via `strokeScaleEnabled: false`. */
const BACKDROP_BORDER = { color: 'rgb(0 0 0 / 25%)', width: 1 } as const

/** The AI-assist ghost overlay (ASSIST.md §5); its frame uses the cursor color when set. */
const GHOST_OPACITY = 0.55
const GHOST_ACCENT = '#4f7cff'
const GHOST_FRAME_DASH = [6, 4] as const

/**
 * A view-only backdrop painted behind the document (theme paper or a checkerboard). It
 * is not part of the {@link Document}: {@link Editor.toPNG} builds a fresh stage without
 * it, as it does without the ghost overlay.
 */
export type CanvasBackdrop = { type: 'color'; color: string } | { type: 'pattern'; image: CanvasImageSource }

function blankDocument(): Document {
    return {
        version: 1,
        width: DEFAULT_CANVAS.width,
        height: DEFAULT_CANVAS.height,
        background: DEFAULT_BACKGROUND,
        layers: [{ id: newId(), name: 'Layer 1', visible: true, opacity: 1, strokes: [] }]
    }
}

/** Clamp a name to the document's rune limit, falling back when empty. */
function clampName(name: string, fallback: string): string {
    const runes = [...name.trim()]
    if (runes.length < 1) return fallback
    return runes.slice(0, LIMITS.maxNameLen).join('')
}

export class Editor {
    private readonly container: HTMLDivElement
    private doc: Document
    private activeLayerId: string
    private activeTool: Tool = penTool
    private style: ToolStyle = { ...DEFAULT_STYLE }

    private readonly history = new History()
    private readonly listeners = new Set<() => void>()

    private stage: Konva.Stage
    /** In-progress gesture points, in logical coords; null when not drawing. */
    private gesture: LogicalPoint[] | null = null
    private previewGroup: Konva.Group | null = null

    /** Resolved color for the cursor ring (a canvas can't read CSS vars); null disables it. */
    private cursorColor: string | null = null
    /** Last hover position in logical coords; null hides the ring. */
    private cursor: { x: number; y: number } | null = null
    private cursorLayer: Konva.Layer | null = null
    private cursorRing: Konva.Circle | null = null

    private backdrop: CanvasBackdrop | null = null
    private backdropLayer: Konva.Layer | null = null
    private backdropRect: Konva.Rect | null = null

    /** The pending AI-assist proposal, kept so {@link rerender} can remount it. */
    private ghostOps: Op[] | null = null
    private ghostLayer: Konva.Layer | null = null

    /** Container size in screen px; {0,0} until the first measure. */
    private viewport = { width: 0, height: 0 }
    private view: ViewState = { zoom: 1, panX: 0, panY: 0 }
    /** While true, a resize re-fits the document; a manual zoom or pan turns it off. */
    private autoFit = true
    private pan: { pointerId: number; startX: number; startY: number; view: ViewState } | null = null
    private readonly resizeObserver: ResizeObserver

    constructor(container: HTMLDivElement, doc?: Document) {
        this.container = container
        this.doc = doc ?? blankDocument()
        const first = this.doc.layers[0]
        this.activeLayerId = first ? first.id : newId()
        this.stage = toKonva(this.doc, container)
        this.bindPointerEvents()
        // Konva only sees releases inside the container; these end a gesture or pan
        // released outside it.
        window.addEventListener('pointerup', this.onWindowPointerUp)
        window.addEventListener('pointercancel', this.onWindowPointerCancel)
        window.addEventListener('keydown', this.onWindowKeyDown)
        container.addEventListener('pointerleave', this.onContainerPointerLeave)
        this.resizeObserver = new ResizeObserver(() => this.measureAndApply())
        this.resizeObserver.observe(container)
        this.measureAndApply()
    }

    // --- public API -----------------------------------------------------------

    setTool(tool: Tool): void {
        this.activeTool = tool
        // A pan tool cannot finish a stroke gesture: drop it rather than half-commit.
        if (tool.kind === 'pan' && this.gesture) {
            this.gesture = null
            this.clearPreview()
        }
        this.syncCursorRing() // the brush ring hides while the hand is active
        this.syncContainerCursor()
    }

    setStyle(patch: Partial<ToolStyle>): void {
        this.style = { ...this.style, ...patch }
        this.syncCursorRing() // the ring's diameter tracks strokeWidth
    }

    /**
     * Enable or re-color the brush-size cursor ring: `strokeWidth` across in logical
     * units, hidden on touch and off-canvas.
     */
    setCursorColor(color: string | null): void {
        this.cursorColor = color
        if (color == null) {
            this.cursorLayer?.destroy()
            this.cursorLayer = null
            this.cursorRing = null
            return
        }
        if (!this.cursorLayer) this.mountCursorOverlay()
        this.syncCursorRing()
    }

    /**
     * Set or clear the {@link CanvasBackdrop}, painted below `doc.background`. A pattern
     * tiles in screen space, so a checkerboard never zooms with the drawing.
     */
    setCanvasBackdrop(backdrop: CanvasBackdrop | null): void {
        this.backdrop = backdrop
        // A fresh rect each time: switching fills on one rect would leak the old attrs.
        this.backdropLayer?.destroy()
        this.backdropLayer = null
        this.backdropRect = null
        if (backdrop != null) this.mountBackdrop()
    }

    getDocument(): Document {
        return this.doc
    }

    loadDocument(doc: Document): void {
        this.doc = doc
        const first = doc.layers[0]
        this.activeLayerId = first ? first.id : newId()
        this.history.clear()
        // Clear the ghost before rerender, or it remounts against the new document.
        this.clearGhost()
        this.autoFit = true
        this.rerender()
        this.fitToViewport()
        this.emitChange()
    }

    toPNG(opts: RenderOptions): Promise<Blob> {
        return renderToPNG(this.doc, opts)
    }

    /** Subscribe to editor-state changes; returns the unsubscribe function. */
    onChange(cb: () => void): () => void {
        this.listeners.add(cb)
        return () => {
            this.listeners.delete(cb)
        }
    }

    // --- view / zoom ----------------------------------------------------------

    /** Current zoom (screen px per logical unit); 1 = 100%. */
    getZoom(): number {
        return this.view.zoom
    }

    /**
     * Map a client point to logical document coords, where a stroke would land. Returns
     * `null` outside the stage container; inside it, coords are raw (unrounded, and may
     * fall outside the document).
     */
    toDocumentCoords(clientX: number, clientY: number): { x: number; y: number } | null {
        // stage.content does not exist headless (tests); the container has the same rect.
        const el = (this.stage.content as HTMLDivElement | undefined) ?? this.container
        const rect = el.getBoundingClientRect()
        const sx = clientX - rect.left
        const sy = clientY - rect.top
        if (sx < 0 || sy < 0 || sx > rect.width || sy > rect.height) return null
        const p = this.stage.getAbsoluteTransform().copy().invert().point({ x: sx, y: sy })
        return { x: p.x, y: p.y }
    }

    /** Scale-to-fit + center the document in the viewport; re-enables auto-fit. */
    fitToViewport(): void {
        if (this.viewport.width <= 0 || this.viewport.height <= 0) return
        this.autoFit = true
        this.view = fitView(this.doc.width, this.doc.height, this.viewport.width, this.viewport.height)
        this.applyView()
        this.emitChange()
    }

    /** Zoom by a multiplicative factor, anchored at a screen point (default: viewport center). */
    zoomBy(factor: number, centerX?: number, centerY?: number): void {
        if (this.viewport.width <= 0 || this.viewport.height <= 0) return
        const cx = centerX ?? this.viewport.width / 2
        const cy = centerY ?? this.viewport.height / 2
        this.autoFit = false
        this.view = zoomAround(this.view, factor, cx, cy)
        this.applyView()
        this.emitChange()
    }

    zoomIn(): void {
        this.zoomBy(ZOOM_STEP)
    }

    zoomOut(): void {
        this.zoomBy(1 / ZOOM_STEP)
    }

    /** Tear down the editor from the host's unmount hook; the instance is unusable afterwards. */
    destroy(): void {
        this.resizeObserver.disconnect()
        window.removeEventListener('pointerup', this.onWindowPointerUp)
        window.removeEventListener('pointercancel', this.onWindowPointerCancel)
        window.removeEventListener('keydown', this.onWindowKeyDown)
        this.container.removeEventListener('pointerleave', this.onContainerPointerLeave)
        // Give the cursor back to the host iff we own it (hand active / mid-pan).
        const cursor = this.container.style.cursor
        if (cursor === 'grab' || cursor === 'grabbing') this.container.style.cursor = ''
        this.listeners.clear()
        this.stage.destroy()
        this.previewGroup = null
        this.cursorLayer = null
        this.cursorRing = null
        this.backdropLayer = null
        this.backdropRect = null
        this.ghostLayer = null
        this.ghostOps = null
        this.gesture = null
        this.pan = null
    }

    // --- history --------------------------------------------------------------

    canUndo(): boolean {
        return this.history.canUndo
    }

    canRedo(): boolean {
        return this.history.canRedo
    }

    undo(): void {
        if (!this.history.undo(this.doc)) return
        this.reconcileActiveLayer()
        this.afterMutation()
    }

    redo(): void {
        if (!this.history.redo(this.doc)) return
        this.reconcileActiveLayer()
        this.afterMutation()
    }

    // --- ai assist (ghost preview; see ASSIST.md §5) --------------------------

    /** Show validated AI-assist {@link Op}s as the ghost overlay, replacing any prior proposal. */
    previewOps(ops: Op[]): void {
        this.clearGhost()
        this.ghostOps = ops
        this.mountGhostOverlay()
    }

    /**
     * Commit the proposal as one composite command, so one Ctrl+Z undoes it. Each
     * `add_layer` gets a fresh id that later `add_stroke`s resolve through a batch-local map.
     * `replace` also removes every existing layer the proposal does not draw into.
     */
    acceptOps(mode: 'add' | 'replace' = 'add'): void {
        const ops = this.ghostOps
        if (ops == null) return
        // An empty composite would push an undo entry that does nothing.
        if (ops.length === 0) {
            this.clearGhost()
            return
        }
        const commands: Command[] = []
        const idMap = new Map<string, string>()
        // addLayerCommand clamps at apply time, so a stale length would collide every new
        // layer (docs/NOTES.md "Multi-layer assist batches need a running insert index").
        let topIndex = this.doc.layers.length
        const drawnInto = new Set<string>()
        let lastAdded: string | undefined
        for (const op of ops) {
            if (op.kind === 'add_layer') {
                const realId = newId()
                idMap.set(op.id, realId)
                lastAdded = realId
                const layer: Layer = {
                    id: realId,
                    name: op.name,
                    visible: true,
                    opacity: 1,
                    strokes: []
                }
                commands.push(addLayerCommand(layer, topIndex))
                topIndex += 1
            } else {
                const resolvedId = idMap.get(op.layerId) ?? op.layerId
                drawnInto.add(resolvedId)
                commands.push(addStrokeCommand(resolvedId, op.stroke))
            }
        }
        if (mode === 'replace') {
            // Removals go after the adds: undo then puts the old layers back in their order.
            for (const layer of this.doc.layers) {
                if (!drawnInto.has(layer.id)) commands.push(removeLayerCommand(this.doc, layer.id))
            }
            if (!drawnInto.has(this.activeLayerId)) {
                this.activeLayerId = lastAdded ?? [...drawnInto][0] ?? this.activeLayerId
            }
        }
        // Before commit: commit rerenders, which remounts the ghost while ghostOps is set.
        this.clearGhost()
        this.commit(compositeCommand(commands, mode === 'replace' ? 'AI replace' : 'AI assist'))
    }

    /** Discard the previewed proposal — nothing enters the document or history. */
    rejectOps(): void {
        if (this.ghostOps == null) return
        this.clearGhost()
        this.stage.batchDraw()
    }

    // --- layers ---------------------------------------------------------------

    getLayers(): LayerView[] {
        return this.doc.layers.map((l) => ({
            id: l.id,
            name: l.name,
            visible: l.visible,
            opacity: l.opacity,
            strokeCount: l.strokes.length
        }))
    }

    getActiveLayerId(): string {
        return this.activeLayerId
    }

    /** Switch the layer new strokes land on. Not undoable: it is editor state. */
    setActiveLayer(id: string): void {
        if (id === this.activeLayerId) return
        if (!this.doc.layers.some((l) => l.id === id)) return
        this.activeLayerId = id
        this.emitChange()
    }

    /** Add an empty layer on top and make it active; returns its id, or null at the layer cap. */
    addLayer(name?: string): string | null {
        if (this.doc.layers.length >= LIMITS.maxLayers) return null
        const fallback = `Layer ${this.doc.layers.length + 1}`
        const layer: Layer = {
            id: newId(),
            name: name === undefined ? fallback : clampName(name, fallback),
            visible: true,
            opacity: 1,
            strokes: []
        }
        this.activeLayerId = layer.id
        this.commit(addLayerCommand(layer, this.doc.layers.length))
        return layer.id
    }

    /** Remove a layer. No-op on the last remaining layer (documents need ≥1). */
    removeLayer(id: string): void {
        if (this.doc.layers.length <= 1) return
        const index = this.doc.layers.findIndex((l) => l.id === id)
        if (index === -1) return
        const wasActive = this.activeLayerId === id
        this.commit(removeLayerCommand(this.doc, id))
        if (wasActive) {
            // Prefer the layer that slid into this slot (was just above), else the top.
            const next = this.doc.layers[index] ?? this.doc.layers[this.doc.layers.length - 1]
            if (next) this.activeLayerId = next.id
        }
    }

    /** Move a layer to a new z-index (0 = bottom). Clamped into range. */
    moveLayer(id: string, toIndex: number): void {
        const from = this.doc.layers.findIndex((l) => l.id === id)
        if (from === -1) return
        const clamped = Math.max(0, Math.min(toIndex, this.doc.layers.length - 1))
        if (clamped === from) return
        this.commit(moveLayerCommand(this.doc, id, clamped))
    }

    renameLayer(id: string, name: string): void {
        const layer = this.doc.layers.find((l) => l.id === id)
        if (!layer) return
        const next = clampName(name, layer.name)
        if (next === layer.name) return
        this.commit(renameLayerCommand(this.doc, id, next))
    }

    setLayerVisible(id: string, visible: boolean): void {
        const layer = this.doc.layers.find((l) => l.id === id)
        if (!layer || layer.visible === visible) return
        this.commit(setLayerVisibleCommand(this.doc, id, visible))
    }

    setLayerOpacity(id: string, opacity: number): void {
        const layer = this.doc.layers.find((l) => l.id === id)
        if (!layer) return
        const clamped = Math.max(0, Math.min(1, opacity))
        if (!Number.isFinite(clamped) || clamped === layer.opacity) return
        this.commit(setLayerOpacityCommand(this.doc, id, clamped))
    }

    // --- internals ------------------------------------------------------------

    private toolContext(): ToolContext {
        return { style: this.style, newId }
    }

    private activeLayer(): Layer | undefined {
        return this.doc.layers.find((l) => l.id === this.activeLayerId)
    }

    private commit(cmd: Command): void {
        this.history.execute(this.doc, cmd)
        this.afterMutation()
    }

    private afterMutation(): void {
        this.rerender()
        this.emitChange()
    }

    private emitChange(): void {
        for (const cb of this.listeners) cb()
    }

    /** After an undo/redo the active layer may have vanished/returned — keep it valid. */
    private reconcileActiveLayer(): void {
        if (this.doc.layers.some((l) => l.id === this.activeLayerId)) return
        const first = this.doc.layers[0]
        if (first) this.activeLayerId = first.id
    }

    private measureAndApply(): void {
        const width = this.container.clientWidth
        const height = this.container.clientHeight
        if (width <= 0 || height <= 0) return
        const changed = width !== this.viewport.width || height !== this.viewport.height
        this.viewport = { width, height }
        if (this.autoFit) {
            this.view = fitView(this.doc.width, this.doc.height, width, height)
        }
        this.applyView()
        if (changed) this.emitChange()
    }

    /** The only writer of the stage transform, so the backdrop's zoom compensation runs here. */
    private applyView(): void {
        const w = this.viewport.width || this.doc.width
        const h = this.viewport.height || this.doc.height
        this.stage.size({ width: w, height: h })
        this.stage.scale({ x: this.view.zoom, y: this.view.zoom })
        this.stage.position({ x: this.view.panX, y: this.view.panY })
        this.syncBackdropScreenSpace()
        this.stage.batchDraw()
    }

    /** Rebuild the stage from the document, keeping the view. */
    private rerender(): void {
        // Chrome dies with the stage; remount what is active
        // (docs/NOTES.md "Overlays must remount inside rerender()").
        this.stage.destroy()
        this.previewGroup = null
        this.cursorLayer = null
        this.cursorRing = null
        this.backdropLayer = null
        this.backdropRect = null
        this.ghostLayer = null
        // A gesture or pan belongs to the old stage; its pointer id can never match again.
        this.gesture = null
        this.pan = null
        this.stage = toKonva(this.doc, this.container)
        this.bindPointerEvents()
        if (this.backdrop != null) this.mountBackdrop()
        if (this.cursorColor != null) {
            this.mountCursorOverlay()
            this.syncCursorRing()
        }
        if (this.ghostOps != null) this.mountGhostOverlay()
        this.applyView()
        this.syncContainerCursor() // clears a stale "grabbing" left by the force-dropped pan
    }

    /** Capture the current pointer position in LOGICAL document coords. */
    private readPoint(evt: Konva.KonvaEventObject<PointerEvent>): LogicalPoint | null {
        const pos = this.stage.getRelativePointerPosition()
        if (!pos) return null
        return { x: pos.x, y: pos.y, pressure: evt.evt.pressure || 0.5 }
    }

    /**
     * Draw the in-flight stroke on the active layer's Konva layer, clipped like a
     * committed one, so it previews exactly as it commits.
     */
    private renderPreview(): void {
        const tool = this.strokeTool()
        if (!this.gesture || !tool) return
        const target = this.activeKonvaLayer()
        if (!target) return
        const stroke = tool.buildStroke(this.toolContext(), this.gesture)

        if (!this.previewGroup) {
            this.previewGroup = new Konva.Group({ listening: false })
            target.add(this.previewGroup)
        }
        this.previewGroup.destroyChildren()
        if (stroke) {
            const preview = toKonva({
                ...this.doc,
                background: null,
                layers: [{ id: 'preview', name: 'preview', visible: true, opacity: 1, strokes: [stroke] }]
            })
            for (const layer of preview.getLayers()) {
                for (const node of layer.getChildren()) node.moveTo(this.previewGroup)
            }
            preview.destroy()
        }
        target.batchDraw()
    }

    /**
     * The stage layer for the active document layer. Stage order is `[backdrop?]
     * [background?] [doc layers...] [overlays]`; this is the one place that offset lives.
     */
    private activeKonvaLayer(): Konva.Layer | null {
        const idx = this.doc.layers.findIndex((l) => l.id === this.activeLayerId)
        if (idx === -1) return null
        const offset = (this.backdropLayer ? 1 : 0) + (this.doc.background != null ? 1 : 0)
        return this.stage.getLayers()[offset + idx] ?? null
    }

    /** Drop the in-flight preview nodes without a full re-render. */
    private clearPreview(): void {
        if (!this.previewGroup) return
        const layer = this.previewGroup.getLayer()
        this.previewGroup.destroy()
        this.previewGroup = null
        layer?.batchDraw()
    }

    /** A gesture may only start inside the document; it may extend past the edge, clipped. */
    private insideDocument(pt: LogicalPoint): boolean {
        return pt.x >= 0 && pt.y >= 0 && pt.x <= this.doc.width && pt.y <= this.doc.height
    }

    /** The active tool when it draws strokes; null while the hand (pan) tool is up. */
    private strokeTool(): StrokeTool | null {
        return this.activeTool.kind === 'stroke' ? this.activeTool : null
    }

    /** Commit (or discard) the in-flight gesture; pt is the final point when known. */
    private finishGesture(pt: LogicalPoint | null): void {
        if (!this.gesture) return
        const tool = this.strokeTool()
        if (!tool) {
            // Defensive: setTool already drops the gesture.
            this.gesture = null
            this.clearPreview()
            return
        }
        if (pt) this.gesture.push(pt)
        const stroke = tool.buildStroke(this.toolContext(), this.gesture)
        this.gesture = null

        if (stroke && this.activeLayer()) {
            this.commit(addStrokeCommand(this.activeLayerId, stroke)) // rerender drops the preview
        } else {
            this.clearPreview()
        }
    }

    // --- canvas backdrop (see setCanvasBackdrop) -------------------------------

    /** Mount the backdrop layer at the very bottom of the current stage. */
    private mountBackdrop(): void {
        if (!this.backdrop) return
        const { width, height } = this.doc
        // Unclipped, unlike projected layers: the shadow and the border's outer half fall
        // outside the doc rect.
        const layer = new Konva.Layer({ listening: false })
        const rect = new Konva.Rect({
            x: 0,
            y: 0,
            width,
            height,
            listening: false,
            shadowColor: BACKDROP_SHADOW.color,
            shadowOpacity: BACKDROP_SHADOW.opacity,
            shadowForStrokeEnabled: false,
            stroke: BACKDROP_BORDER.color,
            strokeWidth: BACKDROP_BORDER.width,
            strokeScaleEnabled: false
        })
        layer.add(rect)
        this.backdropLayer = layer
        this.backdropRect = rect
        if (this.backdrop.type === 'color') {
            rect.fill(this.backdrop.color)
        } else {
            // Typed for image elements, but it only feeds createPattern(), which takes any
            // CanvasImageSource.
            rect.fillPatternImage(this.backdrop.image as HTMLImageElement)
            rect.fillPatternRepeat('repeat')
        }
        this.syncBackdropScreenSpace()
        this.stage.add(layer)
        layer.moveToBottom()
        layer.batchDraw()
    }

    /** Counter-scale the backdrop's screen-space attrs (pattern tiles, shadow) by 1/zoom. */
    private syncBackdropScreenSpace(): void {
        if (!this.backdropRect) return
        const s = 1 / this.view.zoom // zoom is clamped to [MIN_ZOOM, MAX_ZOOM], never 0
        this.backdropRect.shadowBlur(BACKDROP_SHADOW.blur * s)
        this.backdropRect.shadowOffset({
            x: BACKDROP_SHADOW.offset.x * s,
            y: BACKDROP_SHADOW.offset.y * s
        })
        if (this.backdrop?.type === 'pattern') {
            this.backdropRect.fillPatternScale({ x: s, y: s })
        }
    }

    // --- ai assist ghost overlay (see previewOps) ------------------------------

    /**
     * Mount the ghost layer on top, clipped to the doc rect like a committed layer. Only
     * `add_stroke` ops paint.
     */
    private mountGhostOverlay(): void {
        // An empty batch mounts nothing, rather than a bare dashed frame.
        if (this.ghostOps == null || this.ghostOps.length === 0) return
        const { width, height } = this.doc
        const layer = new Konva.Layer({
            listening: false,
            opacity: GHOST_OPACITY,
            clip: { x: 0, y: 0, width, height }
        })
        const strokes: Stroke[] = []
        for (const op of this.ghostOps) {
            if (op.kind === 'add_stroke') strokes.push(op.stroke)
        }
        if (strokes.length > 0) {
            const projected = toKonva({
                ...this.doc,
                background: null,
                layers: [{ id: 'ghost', name: 'ghost', visible: true, opacity: 1, strokes }]
            })
            for (const projectedLayer of projected.getLayers()) {
                for (const node of projectedLayer.getChildren()) node.moveTo(layer)
            }
            projected.destroy()
        }
        // The dashed frame marks a proposal (ASSIST.md §5).
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
                dash: [...GHOST_FRAME_DASH]
            })
        )
        this.ghostLayer = layer
        this.stage.add(layer)
        layer.moveToTop()
        layer.batchDraw()
    }

    /** Drop the ghost overlay and the pending proposal; the document is untouched. */
    private clearGhost(): void {
        this.ghostLayer?.destroy()
        this.ghostLayer = null
        this.ghostOps = null
    }

    // --- cursor ring (see setCursorColor) --------------------------------------

    /** Create the non-listening overlay layer + ring on the CURRENT stage, topmost. */
    private mountCursorOverlay(): void {
        this.cursorLayer = new Konva.Layer({ listening: false })
        this.cursorRing = new Konva.Circle({
            listening: false,
            visible: false,
            strokeWidth: 1,
            // The radius is logical (brush size); the outline stays 1 screen px.
            strokeScaleEnabled: false
        })
        this.cursorLayer.add(this.cursorRing)
        this.stage.add(this.cursorLayer)
    }

    private syncCursorRing(): void {
        if (!this.cursorRing || !this.cursorLayer || this.cursorColor == null) return
        // Hidden while panning or with the hand tool armed: the hand never draws.
        const visible = this.cursor != null && this.pan == null && this.activeTool.kind !== 'pan'
        this.cursorRing.visible(visible)
        if (visible && this.cursor) {
            this.cursorRing.position(this.cursor)
            this.cursorRing.radius(Math.max(this.style.strokeWidth / 2, 0.5))
            this.cursorRing.stroke(this.cursorColor)
        }
        this.cursorLayer.batchDraw()
    }

    /** Track the hover point in logical coords; touch never shows the ring. */
    private trackCursor(evt: Konva.KonvaEventObject<PointerEvent>): void {
        if (!this.cursorRing) return
        // Konva maps touchmove onto "pointermove" too, so a touch can also arrive as a raw
        // TouchEvent with no pointerType.
        const isTouch = evt.evt.type.startsWith('touch') || evt.evt.pointerType === 'touch'
        if (isTouch) {
            this.cursor = null
        } else {
            const pos = this.stage.getRelativePointerPosition()
            this.cursor = pos ? { x: pos.x, y: pos.y } : null
        }
        this.syncCursorRing()
    }

    private readonly onContainerPointerLeave = (): void => {
        this.cursor = null
        this.syncCursorRing()
    }

    private readonly onWindowPointerUp = (): void => {
        this.endPan() // no-op without a pan
        this.finishGesture(null) // no-op when the stage handler already ran
    }

    private readonly onWindowPointerCancel = (): void => {
        this.endPan()
        this.gesture = null
        this.clearPreview()
    }

    private readonly onWindowKeyDown = (e: KeyboardEvent): void => {
        if (e.key === 'Escape') this.endPan()
    }

    // --- pan (middle-button with any tool; primary pointer with the hand tool) --

    /**
     * Anchor a pan at the pointer in screen coords, so the letterbox can be grabbed. The
     * hand tool and the middle button share this path; a second pointer cannot re-anchor.
     */
    private beginPan(pointerId: number): void {
        if (this.pan) return
        const p = this.stage.getPointerPosition()
        if (!p) return
        this.pan = { pointerId, startX: p.x, startY: p.y, view: { ...this.view } }
        this.syncCursorRing() // the brush ring hides for the duration of the pan
        this.syncContainerCursor() // grab → grabbing
    }

    /** Ends a pan from ANY exit (pointerup, window fallback, cancel, Escape); no-op without one. */
    private endPan(): void {
        if (!this.pan) return
        this.pan = null
        this.syncCursorRing()
        this.syncContainerCursor()
    }

    /**
     * Show `grab`/`grabbing` on the container. Only those two values are ever cleared, so
     * a host's own cursor for other tools stays its own.
     */
    private syncContainerCursor(): void {
        const style = this.container.style
        if (this.pan) {
            style.cursor = 'grabbing'
        } else if (this.activeTool.kind === 'pan') {
            style.cursor = 'grab'
        } else if (style.cursor === 'grab' || style.cursor === 'grabbing') {
            style.cursor = ''
        }
    }

    private bindPointerEvents(): void {
        this.stage.on('pointerdown', (evt) => {
            // Pan first: the hand tool pans with any pointer (a raw TouchEvent has no
            // `button`) and the middle button with any tool, letterbox included.
            if (this.activeTool.kind === 'pan' || evt.evt.button === 1) {
                evt.evt.preventDefault()
                this.beginPan(evt.evt.pointerId)
                return
            }
            const pt = this.readPoint(evt)
            if (!pt || !this.insideDocument(pt)) return
            this.gesture = [pt]
            this.renderPreview()
        })

        this.stage.on('pointermove', (evt) => {
            this.trackCursor(evt) // ring follows every non-touch move (pan/hand hides it)
            if (this.pan && evt.evt.pointerId === this.pan.pointerId) {
                const p = this.stage.getPointerPosition()
                if (p) {
                    // A manual view change: stop auto-fit undoing it on the next resize.
                    this.autoFit = false
                    this.view = panBy(this.pan.view, p.x - this.pan.startX, p.y - this.pan.startY)
                    this.applyView()
                }
                return
            }
            if (!this.gesture) return
            const pt = this.readPoint(evt)
            if (!pt) return
            this.gesture.push(pt)
            this.renderPreview()
        })

        this.stage.on('pointerup', (evt) => {
            if (this.pan && evt.evt.pointerId === this.pan.pointerId) {
                this.endPan()
                return
            }
            if (!this.gesture) return
            this.finishGesture(this.readPoint(evt))
        })

        this.stage.on('wheel', (evt) => {
            evt.evt.preventDefault()
            const p = this.stage.getPointerPosition()
            if (!p) return
            const factor = evt.evt.deltaY < 0 ? WHEEL_STEP : 1 / WHEEL_STEP
            this.autoFit = false
            this.view = zoomAround(this.view, factor, p.x, p.y)
            this.applyView()
            this.emitChange()
        })
    }
}
