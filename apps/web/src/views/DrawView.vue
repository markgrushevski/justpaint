<script lang="ts">
/**
 * 8px checkerboard tiles (SVG data-URIs, theme-specific). The two
 * HTMLImageElements are built lazily on first use and shared across mounts.
 */
const GRID_TILE_LIGHT =
    "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='12' y='0' width='12' height='12' fill='%230002'/%3e%3crect x='0' y='12' width='12' height='12' fill='%230002'/%3e%3c/svg%3e"
const GRID_TILE_DARK =
    "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='0' y='0' width='12' height='12' fill='%23fff2'/%3e%3crect x='12' y='12' width='12' height='12' fill='%23fff2'/%3e%3c/svg%3e"

let gridTileLightImg: HTMLImageElement | null = null
let gridTileDarkImg: HTMLImageElement | null = null

function gridTile(dark: boolean): HTMLImageElement {
    if (dark) {
        if (!gridTileDarkImg) {
            gridTileDarkImg = new Image()
            gridTileDarkImg.src = GRID_TILE_DARK
        }
        return gridTileDarkImg
    }
    if (!gridTileLightImg) {
        gridTileLightImg = new Image()
        gridTileLightImg.src = GRID_TILE_LIGHT
    }
    return gridTileLightImg
}
</script>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { OriButton, OriInput, OriSurface, OriToaster, useToast } from '@oriui/vue'
import { useThemeColor } from '@oriui/headless/vue'
import { DEFAULT_CANVAS, DEFAULT_STYLE, DOC_VERSION, Editor, LIMITS, newId, TOOLS } from '@justpaint/editor'
import type { Document, DocSummary, LayerView, Op, ToolId } from '@justpaint/editor'
import {
    copyImage,
    copyText,
    isAuthError,
    isBudgetExhausted,
    isRateLimited,
    toApiError,
    useAssist,
    useAuthGate,
    useGuess,
    useLoadLatestDrawing,
    useSaveDrawing,
    useSessionStore,
    useThemeStore
} from '@core'
import type { Guess } from '@core'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import FloatingToolbar, { TOOL_META } from '../components/FloatingToolbar.vue'
import GuessResult, { type GuessStatus } from '../components/GuessResult.vue'
import LayersPanel from '../components/LayersPanel.vue'
import ShortcutsDialog from '../components/ShortcutsDialog.vue'
import SideMenu from '../components/SideMenu.vue'
import EditorShell from '../components/shell/EditorShell.vue'
import IconButton from '../components/ui/IconButton.vue'

// The shell exposes its Konva mount element; read in onMounted to build the Editor.
const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
// Captured once at mount: blankDocument sizes to it, and it owns the
// coords-readout pointer listeners added/removed below.
let canvasHost: HTMLDivElement | null = null
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

const currentId = ref<string | null>(null)

const DEFAULT_NAME = 'new art'
const drawingName = ref(DEFAULT_NAME)

// Toast duration scales with severity so errors stay readable longer than successes.
const toaster = useToast()
const TOAST_SUCCESS = 3500
const TOAST_INFO = 5000
const TOAST_ERROR = 8000

const saveMutation = useSaveDrawing()
const loadMutation = useLoadLatestDrawing()
const busy = computed(() => saveMutation.isPending.value || loadMutation.isPending.value)

// A batch returned by assist previews as a ghost inside the editor (previewOps)
// and isn't in the document or history until Accept; `pendingOps` is just the
// panel's input/accept-reject phase flag (docs/ASSIST.md §4).
const assistMutation = useAssist()
const assistPending = computed(() => assistMutation.isPending.value)
const assistOpen = ref(false)
const assistPrompt = ref('')
const pendingOps = ref<Op[] | null>(null)
const assistNote = ref<string | null>(null)

// `guessOpen` mounts the card; it can't be the mutation's own `isPending`
// because the card must outlive the request to show the answer it returns.
const guessMutation = useGuess()
const guessOpen = ref(false)
/**
 * What the card shows. `guessResult`/`guessError`/`guessExhausted` are its
 * payloads; only `setGuessStatus` moves them, so a status is never read
 * against a stale payload. Distinct from `guessPending` (the transport's own
 * in-flight flag, which guards spending): they diverge only when the card is
 * dismissed mid-call, where status stays `pending` so reopening resumes the
 * wait instead of restarting an already-paid-for guess.
 */
const guessStatus = ref<GuessStatus>('idle')
const guessPending = computed(() => guessMutation.isPending.value)
const guessResult = ref<Guess | null>(null)
const guessError = ref('')
const guessExhausted = ref(false)
// Set when the document is replaced (New/Load) mid-guess, so a late answer
// about a drawing that's gone is dropped rather than shown. A plain dismiss
// does not set this — that guess is already paid for and still about the
// canvas in front of you.
let guessStale = false

const ui = reactive({
    activeTool: 'pen' as ToolId,
    color: DEFAULT_STYLE.color,
    strokeWidth: DEFAULT_STYLE.strokeWidth,
    fillEnabled: DEFAULT_STYLE.fill !== null,
    fill: DEFAULT_STYLE.fill ?? '#ffffff'
})

// Kept in sync via the editor's onChange subscription so Vue re-renders the
// toolbar, layers panel, zoom and the side menu's canvas-size fields.
const layers = ref<LayerView[]>([])
const activeLayerId = ref('')
const canUndo = ref(false)
const canRedo = ref(false)
const zoom = ref(1)
const zoomPercent = computed(() => Math.round(zoom.value * 100))
// Shown next to the Layers toggle when the panel is closed, since strokes and
// the eraser land on this layer and it must be discoverable without opening it.
const activeLayerName = computed(() => layers.value.find((l) => l.id === activeLayerId.value)?.name ?? '')
// DEFAULT_CANVAS is `as const`; a bare ref() would narrow to the literal.
const docWidth = ref<number>(DEFAULT_CANVAS.width)
const docHeight = ref<number>(DEFAULT_CANVAS.height)
const MAX_LAYERS = LIMITS.maxLayers

const session = useSessionStore()
const gate = useAuthGate()

// A different account signed in mid-visit: the open drawing belongs to the
// previous one, so forget its id. Saving would otherwise PUT a row this user
// doesn't own, and the ownership-scoped query 404s — read as "not found"
// rather than what actually happened.
watch(
    () => session.user?.id,
    (now, before) => {
        if (before && now && now !== before) currentId.value = null
    }
)
const theme = useThemeStore()

// Konva can't read CSS custom properties, so the brush cursor ring gets
// `--ori-color-primary` resolved through the oriui token bridge, re-resolving on
// every theme flip. The editor itself stays token-agnostic — only a color string.
const cursorRingColor = useThemeColor('primary')
watch(cursorRingColor, (color) => editor?.setCursorColor(color || null))

const menuOpen = ref(false)
const shortcutsOpen = ref(false)
// Closed by default on small screens — matches the CSS reflow breakpoint
// (oriui --ori-size-screen_xs, 600px; 601px+ has room for the island).
const layersOpen = ref(window.innerWidth > 600)

const isEmpty = computed(() => layers.value.every((l) => l.strokeCount === 0))

const HINT_KEY = 'jp.hintDismissed'
const hintDismissed = ref(false)
// Hidden once dismissed, once a stroke lands, or while the guess card is up —
// both are centred in the same `#overlay` slot and the card wins the conflict.
const showHint = computed(() => !hintDismissed.value && isEmpty.value && !guessOpen.value)
function dismissHint() {
    hintDismissed.value = true
    try {
        localStorage.setItem(HINT_KEY, '1')
    } catch {
        /* private mode / storage disabled — the hint just won't persist */
    }
}

const confirmNewOpen = ref(false)
const pendingSize = ref<{ w: number; h: number } | null>(null)

// Clearing resets history irreversibly, so confirm only when there's work to
// lose; an already-empty canvas clears straight away.
function requestNew() {
    pendingSize.value = null
    if (isEmpty.value) {
        clearCanvas()
    } else {
        confirmNewOpen.value = true
    }
}

function onApplyCanvasSize(w: number, h: number) {
    if (isEmpty.value) {
        clearCanvas(w, h)
    } else {
        pendingSize.value = { w, h }
        confirmNewOpen.value = true
    }
}

function onConfirmNew() {
    const size = pendingSize.value
    pendingSize.value = null
    clearCanvas(size?.w, size?.h)
    confirmNewOpen.value = false
}

function onCancelNew() {
    pendingSize.value = null
    confirmNewOpen.value = false
}

function clampDim(n: number): number {
    return Math.min(LIMITS.maxCanvasDimension, Math.max(1, Math.round(n)))
}

// Unsized, it matches the current viewport (falling back to DEFAULT_CANVAS
// before layout). Background is null so the view-only paper/checkerboard
// backdrop shows through.
function blankDocument(w?: number, h?: number): Document {
    const el = canvasHost
    const width = clampDim(w ?? (el && el.clientWidth > 0 ? el.clientWidth : DEFAULT_CANVAS.width))
    const height = clampDim(h ?? (el && el.clientHeight > 0 ? el.clientHeight : DEFAULT_CANVAS.height))
    return {
        version: DOC_VERSION,
        width,
        height,
        background: null,
        layers: [{ id: newId(), name: 'Layer 1', visible: true, opacity: 1, strokes: [] }]
    }
}

function syncEditorState() {
    if (!editor) return
    layers.value = editor.getLayers()
    activeLayerId.value = editor.getActiveLayerId()
    canUndo.value = editor.canUndo()
    canRedo.value = editor.canRedo()
    zoom.value = editor.getZoom()
    const doc = editor.getDocument()
    docWidth.value = doc.width
    docHeight.value = doc.height
}

const BACKDROP_KEY = 'jp.backdropGrid'
const backdropGrid = ref(false)

// View-only: the editor guarantees the backdrop never leaks into exports or
// the judged raster. Grid on uses the theme-specific tile; off uses flat paper.
async function applyBackdrop() {
    if (!editor) return
    if (!backdropGrid.value) {
        editor.setCanvasBackdrop({ type: 'color', color: theme.isDark ? '#000000' : '#ffffff' })
        return
    }
    const img = gridTile(theme.isDark)
    if (!img.complete) {
        try {
            await img.decode()
        } catch {
            return // a data-URI that fails to decode won't succeed on retry
        }
        // Re-check the pref/theme still want this tile after the async decode.
        if (!editor || !backdropGrid.value || img !== gridTile(theme.isDark)) return
    }
    editor.setCanvasBackdrop({ type: 'pattern', image: img })
}

watch([() => theme.isDark, backdropGrid], () => void applyBackdrop())

function onToggleGrid(on: boolean) {
    backdropGrid.value = on
    try {
        localStorage.setItem(BACKDROP_KEY, on ? '1' : '0')
    } catch {
        /* private mode / storage disabled — the pref just won't persist */
    }
}

// Pointer's document coordinates, mapped through the editor's stage transform
// (matches where a stroke would land at any zoom/pan). Null when hidden:
// off-canvas, over chrome, or touch (chip is display:none <=600px).
const coords = ref<{ x: number; y: number } | null>(null)
// rAF throttle: coalesce to one read per frame instead of per raw pointermove.
let coordsRaf = 0
let lastPointer: { x: number; y: number } | null = null

function onCanvasPointerMove(e: PointerEvent) {
    lastPointer = { x: e.clientX, y: e.clientY }
    if (coordsRaf) return
    coordsRaf = requestAnimationFrame(() => {
        coordsRaf = 0
        if (!editor || !lastPointer) return
        coords.value = editor.toDocumentCoords(lastPointer.x, lastPointer.y)
    })
}

function onCanvasPointerLeave() {
    if (coordsRaf) {
        cancelAnimationFrame(coordsRaf)
        coordsRaf = 0
    }
    lastPointer = null
    coords.value = null
}

onMounted(() => {
    try {
        hintDismissed.value = localStorage.getItem(HINT_KEY) === '1'
        backdropGrid.value = localStorage.getItem(BACKDROP_KEY) === '1'
    } catch {
        /* private mode / storage disabled — defaults (hint on, paper backdrop) */
    }
    const container = shell.value?.canvasEl ?? null
    if (!container) return
    canvasHost = container
    // The editor sizes its Konva stage to the container and fits the document to
    // it (a ResizeObserver keeps it fitted); it never CSS-transforms the canvas.
    editor = new Editor(container, blankDocument())
    editor.setTool(TOOLS[ui.activeTool])
    editor.setStyle({ ...DEFAULT_STYLE })
    // useThemeColor resolves in its own mounted hook, registered before this one,
    // so the value is usually ready here; the watch above covers late changes.
    editor.setCursorColor(cursorRingColor.value || null)
    void applyBackdrop()
    unsubscribe = editor.onChange(syncEditorState)
    syncEditorState()
    window.addEventListener('keydown', onKeydown)
    container.addEventListener('pointermove', onCanvasPointerMove)
    container.addEventListener('pointerleave', onCanvasPointerLeave)
})

onBeforeUnmount(() => {
    window.removeEventListener('keydown', onKeydown)
    // Tear down any pending AI ghost before the stage is destroyed below.
    clearAssistProposal()
    if (coordsRaf) cancelAnimationFrame(coordsRaf)
    canvasHost?.removeEventListener('pointermove', onCanvasPointerMove)
    canvasHost?.removeEventListener('pointerleave', onCanvasPointerLeave)
    canvasHost = null
    unsubscribe?.()
    unsubscribe = null
    // Destroys the Konva stage and releases its <canvas> elements; dropping the
    // ref alone would leak it (Konva keeps its own module-global registry).
    editor?.destroy()
    editor = null
})

const KEY_TO_TOOL = new Map<string, ToolId>(
    (Object.keys(TOOLS) as ToolId[]).map((id) => [TOOL_META[id].key.toLowerCase(), id])
)

// Shortcuts: Ctrl/Cmd+Z/Y undo-redo, Ctrl/Cmd+0/+/- zoom, Ctrl/Cmd+S save,
// modifier-free tool keys, "?" for the cheat-sheet (docs/DECISIONS.md). Skips
// form fields/contenteditable. The side menu is non-modal — the canvas stays
// interactive behind it — so it does not suppress single keys; only modal
// overlays do.
function onKeydown(e: KeyboardEvent) {
    // The sign-in modal owns the keyboard while open — without this, Ctrl+Z
    // still mutated the canvas behind it and Ctrl+S queued a second save on
    // the same modal, producing two POSTs and two rows from one save.
    if (gate.open) return
    const target = e.target as HTMLElement | null
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
        return
    }
    const key = e.key.toLowerCase()
    if (e.ctrlKey || e.metaKey) {
        if (key === 'z' && !e.shiftKey) {
            e.preventDefault()
            editor?.undo()
        } else if ((key === 'z' && e.shiftKey) || key === 'y') {
            e.preventDefault()
            editor?.redo()
        } else if (key === 's') {
            e.preventDefault() // the browser's own save dialog
            save()
        } else if (key === '0') {
            e.preventDefault()
            editor?.fitToViewport()
        } else if (key === '=' || key === '+') {
            e.preventDefault()
            editor?.zoomIn()
        } else if (key === '-') {
            e.preventDefault()
            editor?.zoomOut()
        }
        return
    }
    if (e.altKey) return
    // Esc order: the non-modal menu first (focus may still sit on the canvas,
    // where its own Esc never fires), then the cheat-sheet, then the guess
    // card — last because it has no backdrop or focus trap, but it is still
    // floating chrome over the drawing so Esc must reach it.
    if (e.key === 'Escape') {
        if (menuOpen.value) menuOpen.value = false
        else if (shortcutsOpen.value) shortcutsOpen.value = false
        else if (guessOpen.value) dismissGuess()
        return
    }
    // Desktop only — the cheat-sheet chip is hidden <=600px.
    if (e.key === '?') {
        if (window.innerWidth <= 600) return
        if (confirmNewOpen.value) return
        e.preventDefault()
        shortcutsOpen.value = !shortcutsOpen.value
        return
    }
    // Every modal overlay must be listed here, or its tool hotkeys leak to this
    // window listener and fire underneath it. The non-modal side menu is not.
    if (shortcutsOpen.value || confirmNewOpen.value) return
    const tool = KEY_TO_TOOL.get(key)
    if (tool) {
        e.preventDefault()
        pickTool(tool)
    }
}

function pickTool(id: ToolId) {
    ui.activeTool = id
    editor?.setTool(TOOLS[id]) // setTool takes the Tool object, not the id
}

function setColor(hex: string) {
    ui.color = hex
    editor?.setStyle({ color: hex })
}

function setWidth(width: number) {
    ui.strokeWidth = width
    editor?.setStyle({ strokeWidth: width })
}

function toggleFill(enabled: boolean) {
    ui.fillEnabled = enabled
    editor?.setStyle({ fill: enabled ? ui.fill : null })
}

function setFill(hex: string) {
    ui.fill = hex
    if (ui.fillEnabled) editor?.setStyle({ fill: hex })
}

function undo() {
    editor?.undo()
}
function redo() {
    editor?.redo()
}

function zoomIn() {
    editor?.zoomIn()
}
function zoomOut() {
    editor?.zoomOut()
}
function fitView() {
    editor?.fitToViewport()
}

function clearCanvas(w?: number, h?: number) {
    if (!editor) return
    // The outgoing document's AI ghost/guess describe a drawing about to be
    // thrown away, so drop both before the fresh blank doc replaces it.
    clearAssistProposal()
    invalidateGuess()
    editor.loadDocument(blankDocument(w, h))
    currentId.value = null
    drawingName.value = DEFAULT_NAME
}

function addLayer() {
    editor?.addLayer()
}
function selectLayer(id: string) {
    editor?.setActiveLayer(id)
}
function removeLayer(id: string) {
    editor?.removeLayer(id)
}
function moveLayer(id: string, toIndex: number) {
    editor?.moveLayer(id, toIndex)
}
function toggleLayerVisible(id: string, visible: boolean) {
    editor?.setLayerVisible(id, visible)
}
function setLayerOpacity(id: string, opacity: number) {
    editor?.setLayerOpacity(id, opacity)
}
function renameLayer(id: string, name: string) {
    editor?.renameLayer(id, name)
}

// Ungated: the name is a local ref until someone saves, and save() is where a
// session is actually needed — a modal here would gate editing a string in memory.
function onRename(name: string) {
    drawingName.value = name.trim() || DEFAULT_NAME
}

async function exportPng() {
    if (!editor) return
    const doc = editor.getDocument()
    const blob = await editor.toPNG({ outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `justpaint-${Date.now()}.png`
    a.click()
    // Revoking in the same tick as click() can abort the download in some browsers.
    setTimeout(() => URL.revokeObjectURL(url), 0)
}

async function copyDocJson() {
    if (!editor) return
    try {
        await copyText(JSON.stringify(editor.getDocument()))
        toaster.success({ text: 'Copied document JSON', duration: TOAST_SUCCESS })
    } catch (err) {
        toaster.error({
            text: err instanceof Error ? err.message : 'Could not copy to the clipboard.',
            duration: TOAST_ERROR
        })
    }
}

async function copyPngToClipboard() {
    if (!editor) return
    try {
        const doc = editor.getDocument()
        const blob = await editor.toPNG({ outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
        await copyImage(blob)
        toaster.success({ text: 'Copied image', duration: TOAST_SUCCESS })
    } catch (err) {
        toaster.error({
            text: err instanceof Error ? err.message : 'Could not copy the image.',
            duration: TOAST_ERROR
        })
    }
}

// Guards a race before the modal is up: `busy` doesn't go true until the
// mutation starts, which is after the gate awaits the store's cookie restore.
// A second trigger in that window would queue a second waiter and one sign-in
// would resolve both, saving the same canvas twice.
let awaitingGate = false

async function gated(reason: string): Promise<boolean> {
    if (awaitingGate) return false
    awaitingGate = true
    try {
        return await gate.ensure(reason)
    } finally {
        awaitingGate = false
    }
}

function reportError(err: unknown, action: string) {
    if (isAuthError(err)) {
        // Ask for a new session but don't replay the action: minutes may have
        // passed and the visitor may sign in as someone else entirely, so
        // re-firing could save something nobody asked to save. Close the
        // cheat-sheet first — its focus trap would fight the incoming dialog.
        shortcutsOpen.value = false
        void gated(`Your session expired — sign in to ${action}.`)
        return
    }
    const api = toApiError(err)
    toaster.error({
        text: api ? `Could not ${action}: ${api.message}` : `Could not ${action} (is the server running?).`,
        duration: TOAST_ERROR
    })
}

async function save() {
    if (!editor || busy.value) return
    if (!(await gated('Sign in to save your drawing.'))) return
    // The modal can stay up for minutes; browser Back can unmount this view
    // and null `editor` underneath us in the meantime.
    if (!editor) return
    const existing = currentId.value
    saveMutation.mutate(
        { id: existing ?? undefined, document: editor.getDocument(), name: drawingName.value },
        {
            onSuccess: (meta) => {
                currentId.value = meta.id
                toaster.success({
                    text: existing ? 'Saved.' : `Saved as ${meta.id}.`,
                    duration: TOAST_SUCCESS
                })
            },
            onError: (err) => reportError(err, 'save')
        }
    )
}

async function load() {
    if (!editor || busy.value) return
    if (!(await gated('Sign in to load your drawing.'))) return
    if (!editor) return
    loadMutation.mutate(undefined, {
        onSuccess: (full) => {
            if (!full) {
                toaster.info({ text: 'No saved drawings yet.', duration: TOAST_INFO })
                return
            }
            editor?.loadDocument(full.document)
            // The old document's AI ghost/guess describe a drawing that's just
            // been replaced, so drop both before the incoming doc lands.
            clearAssistProposal()
            invalidateGuess()
            currentId.value = full.id
            drawingName.value = full.name
            toaster.success({ text: `Loaded ${full.id}.`, duration: TOAST_SUCCESS })
        },
        onError: (err) => reportError(err, 'load')
    })
}

// The minimal summary the endpoint receives (docs/ASSIST.md §4): canvas size
// and the layer inventory, never point paths.
function buildDocSummary(): DocSummary {
    const doc = editor!.getDocument()
    return {
        canvas: { width: doc.width, height: doc.height },
        layers: doc.layers.map((l) => ({ id: l.id, name: l.name, strokeCount: l.strokes.length }))
    }
}

function clearAssistProposal() {
    if (pendingOps.value) editor?.rejectOps()
    pendingOps.value = null
    assistNote.value = null
}

function toggleAssist() {
    assistOpen.value = !assistOpen.value
    if (!assistOpen.value) clearAssistProposal()
}

async function submitAssist() {
    if (!editor) return
    const prompt = assistPrompt.value.trim()
    // Mirrors the submit button's own disabled guard (Enter can reach here too).
    if (!prompt || assistPending.value || pendingOps.value) return
    if (!(await gated('Sign in to use assist.'))) return
    if (!editor) return
    const targetLayerId = editor.getActiveLayerId() || undefined
    assistMutation.mutate(
        { prompt, docSummary: buildDocSummary(), targetLayerId },
        {
            onSuccess: (r) => {
                editor?.previewOps(r.ops)
                pendingOps.value = r.ops
                // Shown inline in the panel; a top-center toast would land over it.
                assistNote.value = r.note ?? null
            },
            onError: (err) => reportError(err, 'use assist')
        }
    )
}

/** Commits the previewed batch as one composite command — one Ctrl+Z undoes it all. */
function acceptAssist() {
    editor?.acceptOps()
    pendingOps.value = null
    assistNote.value = null
    assistPrompt.value = ''
}

function rejectAssist() {
    editor?.rejectOps()
    pendingOps.value = null
    assistNote.value = null
}

/** Moves the card to `status`, dropping every payload with it — the one place
 *  the card's four state pieces change together. */
function setGuessStatus(status: GuessStatus) {
    guessStatus.value = status
    guessResult.value = null
    guessError.value = ''
    guessExhausted.value = false
}

function dismissGuess() {
    guessOpen.value = false
    // A call still in flight keeps `pending`: it's already paid for, so
    // reopening must land back on the wait rather than a blank `idle`.
    if (!guessPending.value) setGuessStatus('idle')
}

// A guess describes the document it was asked about, so drop it (and disown any
// in-flight call) whenever that document is replaced — unlike dismissGuess,
// this resets an in-flight call's status too, since its answer is now about a
// canvas that no longer exists.
function invalidateGuess() {
    guessStale = true
    guessOpen.value = false
    setGuessStatus('idle')
}

// Undoing back to a blank canvas is a document swap in everything but name: an
// answer left standing would violate the guess/hint overlay-slot exclusivity
// and can't be told from a real "draw something first" once the trigger is
// always enabled. A redo pays for a new guess, same as any dismiss always has.
watch(isEmpty, (empty) => {
    if (empty) invalidateGuess()
})

// A failed guess stays in the card, never reportError's toast — with only two
// guesses a day, running out is an expected outcome, not a crash. A lapsed
// session is the exception: it isn't a verdict on the drawing, so it goes
// through the shared gate instead (docs/API.md §3.1 on the two kinds of 429).
function onGuessError(err: unknown) {
    if (guessStale) return
    if (isAuthError(err)) {
        dismissGuess()
        reportError(err, 'guess your drawing')
        return
    }
    setGuessStatus('error')
    const api = toApiError(err)
    if (isBudgetExhausted(err)) {
        guessExhausted.value = true
        guessError.value = api?.message ?? 'That is every AI guess you get today.'
        return
    }
    // The per-IP write-tier 429 clears in seconds and keeps its retry, unlike
    // the daily-budget 429 above; only this side knows waiting will help.
    guessError.value = isRateLimited(err)
        ? `${api?.message ?? 'Too many requests just now'} — try again in a moment.`
        : (api?.message ?? 'The AI could not be reached. Try again.')
}

// Sends the whole document (the server renders it before judging) and is
// capped at a couple of calls a day, so every guard below exists to avoid
// spending one on nothing: a blank canvas, a double-fire, or a dead view.
async function requestGuess() {
    if (!editor || guessPending.value) return
    // A disabled button's tooltip can't reach touch/keyboard, so the card
    // — already the one home for every outcome — states the empty-canvas
    // reason itself instead of just refusing silently.
    if (isEmpty.value) {
        setGuessStatus('idle')
        guessOpen.value = true
        return
    }
    if (!(await gated('Sign in to have the AI guess your drawing.'))) return
    // The modal can stay up for minutes; browser Back can null `editor` (as in save()).
    if (!editor) return
    guessStale = false
    // Open before firing, so the several-second wait has somewhere to live.
    setGuessStatus('pending')
    guessOpen.value = true
    guessMutation.mutate(editor.getDocument(), {
        onSuccess: (result) => {
            if (guessStale) return
            setGuessStatus('answered')
            guessResult.value = result
        },
        onError: onGuessError
    })
}

// A toggle, not a fire button: a second click on an open card hides it instead
// of spending another of the day's calls; re-asking is the card's own "Guess
// again". Anything but `idle` here means an answer nobody has seen yet
// (dismiss resets the status), so reopen onto it rather than paying twice.
function toggleGuess() {
    if (guessOpen.value) {
        dismissGuess()
        return
    }
    if (guessStatus.value !== 'idle') {
        guessOpen.value = true
        return
    }
    void requestGuess()
}
</script>

<template>
    <!-- The shared shell owns the desk, the Konva mount, and the floating-region
         layout; /play composes the same shell with game chrome instead. -->
    <EditorShell ref="shell" mode="draw">
        <template #top-left>
            <OriSurface class="draw__actions">
                <IconButton
                    class="draw__help-btn"
                    icon="help"
                    label="Keyboard shortcuts — ?"
                    placement="bottom"
                    :pressed="shortcutsOpen"
                    @click="shortcutsOpen = !shortcutsOpen"
                />
                <IconButton
                    icon="layers"
                    label="Toggle layers panel"
                    placement="bottom"
                    :pressed="layersOpen"
                    @click="layersOpen = !layersOpen"
                />
                <IconButton
                    icon="assist"
                    label="AI assist — describe what to draw"
                    placement="bottom"
                    :pressed="assistOpen"
                    @click="toggleAssist"
                />
                <!-- Lives in this island, not the top-center strip, because the assist
                     panel already owns that slot. Not disabled on a blank canvas: a
                     disabled button can't show an oriui tooltip on touch, so the guard
                     stays in requestGuess and the card explains the empty-canvas case. -->
                <IconButton
                    icon="guess"
                    label="Guess my drawing — ask the AI what it sees"
                    placement="bottom"
                    :pressed="guessOpen"
                    @click="toggleGuess"
                />
                <!-- Active-layer chip: hidden when the panel is open (it highlights
                     the row itself); clicking here opens the panel. -->
                <button
                    v-if="!layersOpen"
                    class="draw__active-layer"
                    type="button"
                    :aria-label="`Active layer: ${activeLayerName}. Open layers panel`"
                    :title="`Active layer: ${activeLayerName} — click to open layers`"
                    @click="layersOpen = true"
                >
                    {{ activeLayerName }}
                </button>
            </OriSurface>
        </template>

        <!-- The shell's centering strip is pointer-events:none; the panel opts back
             in. Flips from input to accept/reject while a proposal is pending. -->
        <template #top-center>
            <OriSurface v-if="assistOpen" class="draw__assist" role="group" aria-label="AI assist">
                <!-- Explicit close: the actions-island toggle can be off-screen on
                     narrow widths, so the panel stays dismissible from within. -->
                <div class="draw__assist-head">
                    <span class="draw__assist-title">AI assist</span>
                    <IconButton icon="close" label="Close AI assist" placement="bottom" @click="toggleAssist" />
                </div>
                <template v-if="pendingOps">
                    <p v-if="assistNote" class="draw__assist-note">{{ assistNote }}</p>
                    <div class="draw__assist-actions">
                        <OriButton variant="fill" radius="md" text="Accept" fluid @click="acceptAssist" />
                        <OriButton variant="outline" radius="md" text="Reject" fluid @click="rejectAssist" />
                    </div>
                </template>
                <template v-else>
                    <div class="draw__assist-row">
                        <OriInput
                            v-model="assistPrompt"
                            class="draw__assist-input"
                            aria-label="Describe what to draw"
                            placeholder="Describe what to draw…"
                            :disabled="assistPending"
                            @keydown.enter="submitAssist"
                        />
                        <OriButton
                            variant="fill"
                            radius="md"
                            text="Draw"
                            :loading="assistPending"
                            :disabled="!assistPrompt.trim() || assistPending"
                            @click="submitAssist"
                        />
                    </div>
                </template>
            </OriSurface>
        </template>

        <!-- The shell's centering strip is pointer-events:none; the toolbar opts
             back in so drawing passes through the flanks beside it. -->
        <template #bottom-center>
            <FloatingToolbar
                class="draw__toolbar-item"
                :active-tool="ui.activeTool"
                :color="ui.color"
                :stroke-width="ui.strokeWidth"
                :fill-enabled="ui.fillEnabled"
                :fill="ui.fill"
                :can-undo="canUndo"
                :can-redo="canRedo"
                @pick-tool="pickTool"
                @set-color="setColor"
                @set-width="setWidth"
                @toggle-fill="toggleFill"
                @set-fill="setFill"
                @undo="undo"
                @redo="redo"
            />
        </template>

        <!-- Default placement="top" suits a bottom-edge island; no override needed. -->
        <template #bottom-right>
            <OriSurface class="draw__zoom" role="group" aria-label="Zoom">
                <IconButton icon="minus" label="Zoom out — Ctrl+-" @click="zoomOut" />
                <span class="draw__zoom-value">{{ zoomPercent }}%</span>
                <IconButton icon="plus" label="Zoom in — Ctrl+=" @click="zoomIn" />
                <IconButton icon="fit" label="Fit — Ctrl+0" @click="fitView" />
            </OriSurface>
        </template>

        <template #bottom-left>
            <OriSurface v-if="coords" class="draw__coords">
                <span class="draw__coords-mark" aria-hidden="true">⌖</span>
                <span class="draw__coords-value">{{ Math.round(coords.x) }}, {{ Math.round(coords.y) }}</span>
            </OriSurface>
        </template>

        <!-- All but EmptyState teleport to body or manage their own stacking; the
             shell overlay is pointer-events:none, so each opts back in as needed. -->
        <template #overlay>
            <OriToaster position="top-center" align="center" />

            <Transition name="jp-pop">
                <EmptyState
                    v-if="showHint"
                    class="draw__empty"
                    :signed-in="session.isLoggedIn"
                    @dismiss="dismissHint"
                    @sign-in="void gated('Sign in to save and load your drawings.')"
                    @shortcuts="shortcutsOpen = true"
                />
            </Transition>

            <!-- The one surface for guess: wait, answer, every failure, and the
                 empty-canvas message. showHint stands down whenever this is open. -->
            <Transition name="jp-pop">
                <GuessResult
                    v-if="guessOpen"
                    class="draw__guess"
                    :status="guessStatus"
                    :label="guessResult?.label ?? null"
                    :confidence="guessResult?.confidence ?? 0"
                    :alternatives="guessResult?.alternatives ?? []"
                    :error="guessError"
                    :exhausted="guessExhausted"
                    :can-retry="!isEmpty"
                    @again="requestGuess"
                    @dismiss="dismissGuess"
                />
            </Transition>

            <ShortcutsDialog :open="shortcutsOpen" @close="shortcutsOpen = false" />

            <ConfirmDialog
                :open="confirmNewOpen"
                title="Clear the canvas?"
                message="This starts a new drawing and can't be undone."
                confirm-text="Clear"
                cancel-text="Cancel"
                danger
                @confirm="onConfirmNew"
                @cancel="onCancelNew"
            />
        </template>

        <!-- Self-teleports to body; non-modal, canvas stays live. -->
        <template #drawer>
            <SideMenu
                :open="menuOpen"
                :busy="busy"
                :title="drawingName"
                :backdrop-grid="backdropGrid"
                :canvas-width="docWidth"
                :canvas-height="docHeight"
                @close="menuOpen = false"
                @new-drawing="requestNew"
                @load="load"
                @save="save"
                @export-png="exportPng"
                @copy-text="copyDocJson"
                @copy-image="copyPngToClipboard"
                @rename="onRename"
                @toggle-grid="onToggleGrid"
                @apply-canvas-size="onApplyCanvasSize"
            />
        </template>

        <!-- Free-floating /draw chrome, self-positioned as direct children of the
             shell's default (non-stacking-context) slot. -->

        <!-- z-110, above the drawer's z-100, so the same chip stays clickable to
             close it; tooltip drops below to stay on-screen at the top edge. -->
        <OriSurface class="draw__menu-toggle">
            <IconButton
                :icon="menuOpen ? 'close' : 'menu'"
                :label="menuOpen ? 'Close menu' : 'Open menu'"
                placement="bottom"
                :pressed="menuOpen"
                @click="menuOpen = !menuOpen"
            />
        </OriSurface>

        <!-- Phones only: the toolbar hides its history group <=600px, so this
             keeps undo/redo a one-tap home clear of the tool row. -->
        <OriSurface class="draw__history" role="group" aria-label="History">
            <IconButton icon="undo" label="Undo" :disabled="!canUndo" @click="undo" />
            <IconButton icon="redo" label="Redo" :disabled="!canRedo" @click="redo" />
        </OriSurface>

        <!-- Scrim behind the mobile layers bottom sheet (display:none >600px) -->
        <div v-if="layersOpen" class="draw__layers-scrim" @click="layersOpen = false"></div>

        <!-- Layers: a dropdown under the actions island (desktop), bottom sheet (phones) -->
        <div v-show="layersOpen" class="draw__layers">
            <LayersPanel
                :layers="layers"
                :active-layer-id="activeLayerId"
                :can-add="layers.length < MAX_LAYERS"
                @add="addLayer"
                @select="selectLayer"
                @remove="removeLayer"
                @move="moveLayer"
                @toggle-visible="toggleLayerVisible"
                @set-opacity="setLayerOpacity"
                @rename="renameLayer"
                @close="layersOpen = false"
            />
        </div>
    </EditorShell>
</template>

<style scoped>
/* The desk, Konva mount and floating-region positioning live in EditorShell
   (the shared /draw+/play skeleton); what remains here is /draw's own chrome. */

.draw__menu-toggle {
    position: absolute;
    top: var(--ori-size-gap_md, 0.5rem);
    right: var(--ori-size-gap_md, 0.5rem);
    z-index: 110;

    padding: var(--ori-size-gap_xs, 0.125rem);
}

.draw__actions {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
    flex-wrap: wrap;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

/* Neutral structural hover only (docs/DESIGN-SYSTEM.md §1), never a brand-role
   mix; the global focus-visible ring covers keyboard. */
.draw__active-layer {
    max-width: 8rem;
    padding: 0.15rem 0.4rem;

    border: none;
    border-radius: var(--ori-size-radius_sm, 4px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-family: inherit;
    font-size: var(--ori-font-size_sm, 0.85rem);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;

    cursor: pointer;
}

.draw__active-layer:hover {
    background-color: var(--jp-neutral-hover-bg, color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent));
}

/* EditorShell's bottom-center strip is pointer-events:none (its empty flanks
   never eat canvas events) — the toolbar opts back in so it stays interactive. */
.draw__toolbar-item {
    pointer-events: auto;
}

/* Like the toolbar, the top-center strip is pointer-events:none, so the panel
   opts back in. Clamped so it never spills past the viewport on a phone. */
.draw__assist {
    pointer-events: auto;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: min(30rem, calc(100vw - 1.5rem));
    padding: var(--ori-size-gap_sm, 0.25rem);
}

.draw__assist-row {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.draw__assist-input {
    flex: 1;
    min-width: 0;
}

.draw__assist-actions {
    display: flex;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.draw__assist-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.draw__assist-title {
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-weight: 600;
}

.draw__assist-note {
    margin: 0;
    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
}

/* EditorShell's overlay layer is full-bleed but pointer-events:none so it never
   blocks drawing; only the card (pointer-events:auto) is interactive. */
.draw__empty {
    pointer-events: auto;
}

/* The card owns its own width/scroll/pointer-events; /draw's business is WHERE
   it sits (the small-screen rule below). The lift is named here because the
   overlay centers its children and the toolbar owns the bottom ~4rem of a
   small screen — 5rem of margin buys a ~2.5rem rise so tools stay tappable. */
.draw__guess {
    --jp-guess-lift: 5rem;
}

/* The card fades + settles in place (a straight fade/scale — NOT the toast's
   horizontal slide, which reads wrong on a centered welcome card). */
.jp-pop-enter-active,
.jp-pop-leave-active {
    transition:
        opacity 0.18s ease-out,
        transform 0.18s ease-out;
}

.jp-pop-enter-from,
.jp-pop-leave-to {
    opacity: 0;
    transform: scale(0.96);
}

/* The bottom-right zoom island — EditorShell's bottom-right region positions it. */
.draw__zoom {
    display: flex;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

.draw__zoom-value {
    min-width: 3.1rem;
    padding: 0.25rem;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-variant-numeric: tabular-nums;
    text-align: center;
}

/* The bottom-left region is itself pointer-events:none; this stays a passive
   readout too, so it never intercepts canvas drawing at any zoom/pan. */
.draw__coords {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_xs, 0.125rem);

    padding: 0.2rem 0.5rem;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-variant-numeric: tabular-nums;

    opacity: 0.7;
    pointer-events: none;
    user-select: none;
}

.draw__coords-mark {
    font-size: 0.9em;
    opacity: 0.8;
}

/* display:none here so it can never show on desktop; the media block below
   flips it on. */
.draw__history {
    position: absolute;

    /* Second row: the top row belongs to the actions island + toggler, which
       can stretch across a narrow phone — same-row placement overlapped them
       (verified at 405px). 3.125rem = the 50px top-row island height. */
    top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + 3.125rem);
    left: var(--ori-size-gap_md, 0.5rem);
    z-index: 10;

    display: none;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

/* The wrapper stretches the panel to its clamped height so its own list scrolls. */
.draw__layers {
    position: absolute;
    top: calc(
        var(--ori-size-gap_md, 0.5rem) + var(--ori-size-action_md, 2.75rem) + var(--ori-size-gap_sm, 0.25rem) + 0.35rem
    );
    left: var(--ori-size-gap_md, 0.5rem);
    z-index: 9;

    display: flex;
    align-items: stretch;

    width: 16rem;
    max-height: calc(100dvh - 10rem);
}

/* Scrim behind the mobile layers sheet — display:none here so it can never
   show on desktop; the <=600px media query turns it on. */
.draw__layers-scrim {
    position: fixed;
    inset: 0;
    z-index: 59;

    display: none;

    background-color: rgb(0 0 0 / 35%);
}

.toast-enter-active,
.toast-leave-active {
    transition:
        opacity 180ms ease,
        transform 180ms ease;
}

.toast-enter-from,
.toast-leave-to {
    opacity: 0;
    transform: translateX(-50%) translateY(-0.4rem);
}

/* The assist panel drops to its own full-width row so its opaque surface can
   never cover the actions island (holds its only external close affordance)
   or the menu toggle. Breakpoint = 1050px, not 768px: the panel is a centered
   ~30rem box, and its left edge reaches the actions island until the viewport
   is this wide (measured overlap persists to ~1050px, verified at 800px). */
@media (width <= 1050px) {
    .draw__assist {
        position: absolute;
        /* Region top is already offset by gap_md, so +gap_md lands the panel on
           the same visual row as .draw__history (2*gap_md + island height). */
        top: calc(var(--ori-size-gap_md, 0.5rem) + 3.125rem);
        left: var(--ori-size-gap_md, 0.5rem);
        right: var(--ori-size-gap_md, 0.5rem);

        width: auto;
    }
}

/* Keyed on both axes: a width-only rule misses a phone in landscape (667x375),
   where a full answer (label + certainty + two runner-ups) leaves single-digit
   pixels above the toolbar. Measured with the lift on: card ends at 231, bar
   starts at 309 — 500px height is where the two start competing for space. */
@media (width <= 600px), (height <= 500px) {
    .draw__guess {
        margin-bottom: var(--jp-guess-lift);
    }
}

@media (width <= 600px) {
    /* The mobile history island claims that second row (top-left), so the assist
       panel drops one row further to clear it. */
    .draw__assist {
        top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + 3.125rem * 2);
    }

    .draw__layers-scrim {
        display: block;
    }

    /* The layers island becomes a full-width bottom sheet over the scrim. The
       wrapper carries the sheet chrome (top radius + surface) so the panel's
       own rounded bottom corners can't notch the screen edge. */
    .draw__layers {
        position: fixed;
        inset: auto 0 0;
        z-index: 60;

        width: 100%;
        max-height: 60dvh;
        overflow: hidden;

        border-radius: var(--ori-size-radius_lg, 12px) var(--ori-size-radius_lg, 12px) 0 0;
        background-color: var(--ori-color-surface);
    }

    /* Undo/redo island on — the toolbar hides its own history group here. */
    .draw__history {
        display: flex;
    }

    /* The Help button goes on phones (no hardware keyboard); Layers stays in the
       same island. The class rides the IconButton directly, so display:none
       drops the whole control (no empty flex gap). */
    .draw__help-btn {
        display: none;
    }

    /* No hover on touch — the coordinate readout has nothing to track. */
    .draw__coords {
        display: none;
    }
}
</style>
