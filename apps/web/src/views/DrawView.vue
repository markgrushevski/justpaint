<script lang="ts">
/** 8px checkerboard tiles per theme; the images are built lazily and shared across mounts. */
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
import type { Document, LayerView, Op, ToolId } from '@justpaint/editor'
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

const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
// Captured at mount: blankDocument sizes to it, and the coords listeners live on it.
let canvasHost: HTMLDivElement | null = null
let editor: Editor | null = null
let unsubscribe: (() => void) | null = null

const currentId = ref<string | null>(null)

const DEFAULT_NAME = 'new art'
const drawingName = ref(DEFAULT_NAME)

const toaster = useToast()
const TOAST_SUCCESS = 3500
const TOAST_INFO = 5000
const TOAST_ERROR = 8000

const saveMutation = useSaveDrawing()
const loadMutation = useLoadLatestDrawing()
const busy = computed(() => saveMutation.isPending.value || loadMutation.isPending.value)

// An assist batch is a ghost in the editor until Accept; `pendingOps` only flags the
// panel's accept/reject phase.
const assistMutation = useAssist()
const assistPending = computed(() => assistMutation.isPending.value)
const assistOpen = ref(false)
const assistPrompt = ref('')
const pendingOps = ref<Op[] | null>(null)
const assistNote = ref<string | null>(null)

// Not the mutation's `isPending`: the card outlives the request to show its answer.
const guessMutation = useGuess()
const guessOpen = ref(false)
/**
 * What the card shows; only `setGuessStatus` moves it, together with the payloads below.
 * It differs from `guessPending` only after a mid-call dismiss: the status stays
 * `pending`, so reopening resumes the already-paid-for wait.
 */
const guessStatus = ref<GuessStatus>('idle')
const guessPending = computed(() => guessMutation.isPending.value)
const guessResult = ref<Guess | null>(null)
const guessError = ref('')
const guessExhausted = ref(false)
// Set when New or Load replaces the document mid-guess, so a late answer is dropped. A
// plain dismiss does not set it: that answer is still about the canvas on screen.
let guessStale = false

const ui = reactive({
    activeTool: 'pen' as ToolId,
    color: DEFAULT_STYLE.color,
    strokeWidth: DEFAULT_STYLE.strokeWidth,
    fillEnabled: DEFAULT_STYLE.fill !== null,
    fill: DEFAULT_STYLE.fill ?? '#ffffff'
})

// Mirrors of editor state, refreshed through its onChange subscription.
const layers = ref<LayerView[]>([])
const activeLayerId = ref('')
const canUndo = ref(false)
const canRedo = ref(false)
const zoom = ref(1)
const zoomPercent = computed(() => Math.round(zoom.value * 100))
// Shown by the Layers toggle while the panel is closed, since new strokes land there.
const activeLayerName = computed(() => layers.value.find((l) => l.id === activeLayerId.value)?.name ?? '')
// DEFAULT_CANVAS is `as const`; a bare ref() would narrow to the literal.
const docWidth = ref<number>(DEFAULT_CANVAS.width)
const docHeight = ref<number>(DEFAULT_CANVAS.height)
const MAX_LAYERS = LIMITS.maxLayers

const session = useSessionStore()
const gate = useAuthGate()

// Another account signed in: the open drawing is the previous one's, and saving it
// would 404 on the ownership-scoped PUT.
watch(
    () => session.user?.id,
    (now, before) => {
        if (before && now && now !== before) currentId.value = null
    }
)
const theme = useThemeStore()

// Konva can't read CSS variables, so the ring gets the primary token resolved on every
// theme flip.
const cursorRingColor = useThemeColor('primary')
watch(cursorRingColor, (color) => editor?.setCursorColor(color || null))

const menuOpen = ref(false)
const shortcutsOpen = ref(false)
// Closed by default at the 600px reflow breakpoint (oriui --ori-size-screen_xs).
const layersOpen = ref(window.innerWidth > 600)

const isEmpty = computed(() => layers.value.every((l) => l.strokeCount === 0))

const HINT_KEY = 'jp.hintDismissed'
const hintDismissed = ref(false)
// The hint and the guess card share the `#overlay` slot; the card wins.
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

// Clearing drops history, so confirm only when there is work to lose.
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

// Unsized, it fits the viewport. A null background lets the view-only backdrop show.
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

// View-only: the editor keeps the backdrop out of exports and the judged raster.
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
        // The pref or theme may have changed during the decode.
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

// Pointer position in document coords; null off-canvas or over chrome.
const coords = ref<{ x: number; y: number } | null>(null)
// One read per animation frame, not per pointermove.
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
    editor = new Editor(container, blankDocument())
    editor.setTool(TOOLS[ui.activeTool])
    editor.setStyle({ ...DEFAULT_STYLE })
    // useThemeColor resolves in an earlier mounted hook; the watch covers late changes.
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
    // Dropping the ref alone would leak the stage: Konva keeps a module-global registry.
    editor?.destroy()
    editor = null
})

const KEY_TO_TOOL = new Map<string, ToolId>(
    (Object.keys(TOOLS) as ToolId[]).map((id) => [TOOL_META[id].key.toLowerCase(), id])
)

// Skips form fields. The side menu is non-modal, so it does not suppress single keys;
// only modal overlays do.
function onKeydown(e: KeyboardEvent) {
    // The sign-in modal owns the keyboard: Ctrl+S behind it would queue a second save.
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
    // The menu first, since its own Esc never fires while focus is on the canvas; then
    // the cheat-sheet, then the guess card.
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
    // Every modal overlay must be listed here, or tool hotkeys fire underneath it.
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
    // The ghost and the guess describe the outgoing drawing.
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

// Ungated: only save() needs a session.
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

// Covers the gap before `busy` turns true, while the gate awaits the cookie restore: a
// second trigger there would queue a second waiter, and one sign-in would save twice.
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
        // Don't replay the action: the visitor may sign in as someone else. Close the
        // cheat-sheet first, or its focus trap fights the dialog.
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
    // Browser Back can unmount the view while the modal is up.
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
            // The ghost and the guess describe the replaced drawing.
            clearAssistProposal()
            invalidateGuess()
            currentId.value = full.id
            drawingName.value = full.name
            toaster.success({ text: `Loaded ${full.id}.`, duration: TOAST_SUCCESS })
        },
        onError: (err) => reportError(err, 'load')
    })
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
        { prompt, document: editor.getDocument(), targetLayerId },
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

// `replace` swaps the whole drawing for the proposal; one Ctrl+Z restores it.
function acceptAssist(mode: 'add' | 'replace') {
    editor?.acceptOps(mode)
    pendingOps.value = null
    assistNote.value = null
    assistPrompt.value = ''
}

function rejectAssist() {
    editor?.rejectOps()
    pendingOps.value = null
    assistNote.value = null
}

/** Move the card to `status`, clearing every payload with it. */
function setGuessStatus(status: GuessStatus) {
    guessStatus.value = status
    guessResult.value = null
    guessError.value = ''
    guessExhausted.value = false
}

function dismissGuess() {
    guessOpen.value = false
    // An in-flight call keeps `pending`, so reopening lands back on the paid-for wait.
    if (!guessPending.value) setGuessStatus('idle')
}

// Unlike dismissGuess, this also disowns an in-flight call: its answer is about a
// canvas that no longer exists.
function invalidateGuess() {
    guessStale = true
    guessOpen.value = false
    setGuessStatus('idle')
}

// Undoing to a blank canvas is a document swap too, and a standing answer would share
// the overlay slot with the hint.
watch(isEmpty, (empty) => {
    if (empty) invalidateGuess()
})

// Failures stay in the card, not a toast: with two guesses a day, running out is
// expected. A lapsed session goes through the auth gate instead.
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
    // Unlike the daily budget, the per-IP 429 clears in seconds (docs/API.md §3.1).
    guessError.value = isRateLimited(err)
        ? `${api?.message ?? 'Too many requests just now'} — try again in a moment.`
        : (api?.message ?? 'The AI could not be reached. Try again.')
}

// A couple of guesses a day, so every guard below avoids spending one on nothing.
async function requestGuess() {
    if (!editor || guessPending.value) return
    // A disabled button's tooltip can't reach touch or keyboard, so the card explains.
    if (isEmpty.value) {
        setGuessStatus('idle')
        guessOpen.value = true
        return
    }
    if (!(await gated('Sign in to have the AI guess your drawing.'))) return
    // Browser Back can unmount the view while the modal is up.
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

// A toggle: a second click hides the card rather than paying again, and a non-idle
// status is an unseen answer to reopen onto.
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
                <!-- Not disabled on a blank canvas: requestGuess explains it in the card. -->
                <IconButton
                    icon="guess"
                    label="Guess my drawing — ask the AI what it sees"
                    placement="bottom"
                    :pressed="guessOpen"
                    @click="toggleGuess"
                />
                <!-- Hidden while the panel is open; the panel highlights the row itself. -->
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

        <!-- Flips from input to accept/reject while a proposal is pending. -->
        <template #top-center>
            <OriSurface v-if="assistOpen" class="draw__assist" role="group" aria-label="AI assist">
                <!-- The actions-island toggle can be off-screen on narrow widths. -->
                <div class="draw__assist-head">
                    <span class="draw__assist-title">AI assist</span>
                    <IconButton icon="close" label="Close AI assist" placement="bottom" @click="toggleAssist" />
                </div>
                <template v-if="pendingOps">
                    <p v-if="assistNote" class="draw__assist-note">{{ assistNote }}</p>
                    <div class="draw__assist-actions">
                        <OriButton
                            variant="fill"
                            radius="md"
                            :text="isEmpty ? 'Accept' : 'Add on top'"
                            fluid
                            @click="acceptAssist('add')"
                        />
                        <OriButton
                            v-if="!isEmpty"
                            variant="outline"
                            radius="md"
                            text="Replace drawing"
                            fluid
                            @click="acceptAssist('replace')"
                        />
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

        <!-- Self-positioned chrome in the shell's default slot. The toggle sits above
             the drawer, so the same chip closes it. -->
        <OriSurface class="draw__menu-toggle">
            <IconButton
                :icon="menuOpen ? 'close' : 'menu'"
                :label="menuOpen ? 'Close menu' : 'Open menu'"
                placement="bottom"
                :pressed="menuOpen"
                @click="menuOpen = !menuOpen"
            />
        </OriSurface>

        <!-- Phones only: the toolbar hides its history group <=600px. -->
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

/* Neutral structural hover only, never a brand-role mix (docs/DESIGN-SYSTEM.md §1). */
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

/* The shell's strips are pointer-events:none, so chrome in them opts back in. */
.draw__toolbar-item {
    pointer-events: auto;
}

/* Clamped so it never spills past a phone's viewport. */
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
    flex-wrap: wrap;
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

.draw__empty {
    pointer-events: auto;
}

/* The card sizes itself; /draw only places it. On a small screen the lift clears the
   ~4rem toolbar. */
.draw__guess {
    --jp-guess-lift: 5rem;
}

/* A plain fade and scale; the toast's slide reads wrong on a centered card. */
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

/* A passive readout that never intercepts drawing. */
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

/* Hidden by default; the <=600px block shows it. */
.draw__history {
    position: absolute;

    /* The second row: the top row's islands can span a narrow phone. 3.125rem is the
       50px top-row island height. */
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

/* Hidden by default; the <=600px block shows it. */
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

/* Below ~1050px the centered 30rem assist panel reaches the actions island, which
   holds its only outside close, so it drops to its own full-width row. */
@media (width <= 1050px) {
    .draw__assist {
        position: absolute;
        /* The region is already offset by gap_md: this is .draw__history's row. */
        top: calc(var(--ori-size-gap_md, 0.5rem) + 3.125rem);
        left: var(--ori-size-gap_md, 0.5rem);
        right: var(--ori-size-gap_md, 0.5rem);

        width: auto;
    }
}

/* Height too: on a landscape phone a full answer ends pixels above the toolbar. */
@media (width <= 600px), (height <= 500px) {
    .draw__guess {
        margin-bottom: var(--jp-guess-lift);
    }
}

@media (width <= 600px) {
    /* The history island takes the second row, so the assist panel drops one more. */
    .draw__assist {
        top: calc(var(--ori-size-gap_md, 0.5rem) * 2 + 3.125rem * 2);
    }

    .draw__layers-scrim {
        display: block;
    }

    /* A full-width bottom sheet. The wrapper carries the sheet chrome, so the panel's
       rounded bottom corners can't notch the screen edge. */
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

    .draw__history {
        display: flex;
    }

    /* Phones have no keyboard, so no shortcuts help. */
    .draw__help-btn {
        display: none;
    }

    /* No hover on touch, so nothing to track. */
    .draw__coords {
        display: none;
    }
}
</style>
