<script lang="ts" setup>
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Editor, TOOLS, DEFAULT_STYLE, newId } from '@justpaint/editor'
import type { ToolId } from '@justpaint/editor'
import type { Document } from '@justpaint/document'
import { DEFAULT_BACKGROUND, DOC_VERSION, parseDocument } from '@justpaint/document'
import { drawings, isAuthError, toApiError } from '@core'
import EditorToolbar from '../components/EditorToolbar.vue'

const containerRef = ref<HTMLDivElement | null>(null)
let editor: Editor | null = null

/** Id of the drawing currently open (set after a successful save/load). */
const currentId = ref<string | null>(null)
const busy = ref(false)
const message = ref<string | null>(null)

const ui = reactive({
    activeTool: 'pen' as ToolId,
    color: DEFAULT_STYLE.color,
    strokeWidth: DEFAULT_STYLE.strokeWidth,
    fillEnabled: DEFAULT_STYLE.fill !== null,
    fill: DEFAULT_STYLE.fill ?? '#ffffff'
})

// A modest working canvas: the editor has no fit-to-viewport / zoom yet (Phase 2),
// and the spec default (DEFAULT_CANVAS, 1920x1080) overflows most viewports.
const CANVAS = { width: 1280, height: 720 }

function blankDocument(): Document {
    return {
        version: DOC_VERSION,
        width: CANVAS.width,
        height: CANVAS.height,
        background: DEFAULT_BACKGROUND,
        layers: [{ id: newId(), name: 'Layer 1', visible: true, opacity: 1, strokes: [] }]
    }
}

onMounted(() => {
    if (!containerRef.value) return
    editor = new Editor(containerRef.value, parseDocument(blankDocument()))
    editor.setTool(TOOLS[ui.activeTool])
    editor.setStyle({ ...DEFAULT_STYLE })
})

onBeforeUnmount(() => {
    // Destroy the Konva stage (removes it from Konva's module-global registry and
    // releases its <canvas> elements); merely dropping the ref would leak it.
    editor?.destroy()
    editor = null
})

/* --- toolbar handlers ------------------------------------------------ */

function pickTool(id: ToolId) {
    ui.activeTool = id
    // setTool takes a Tool OBJECT, not the id string.
    editor?.setTool(TOOLS[id])
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

function clearCanvas() {
    if (!editor) return
    // Validate the freshly built blank doc before loading (loadDocument does
    // not validate). parseDocument throws DocumentValidationError on bad input.
    editor.loadDocument(parseDocument(blankDocument()))
    currentId.value = null
    message.value = null
}

async function exportPng() {
    if (!editor) return
    const blob = await editor.toPNG({ outWidth: CANVAS.width, outHeight: CANVAS.height, fit: 'contain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `justpaint-${Date.now()}.png`
    a.click()
    // Defer the revoke a tick: revoking in the same tick as click() can abort
    // the download in some browsers before the navigation resolves.
    setTimeout(() => URL.revokeObjectURL(url), 0)
}

function reportError(err: unknown, action: string) {
    if (isAuthError(err)) {
        message.value = `Sign in to ${action} (auth not wired yet).`
        return
    }
    const api = toApiError(err)
    message.value = api ? `Could not ${action}: ${api.message}` : `Could not ${action} (is the server running?).`
}

async function save() {
    if (!editor || busy.value) return
    busy.value = true
    message.value = null
    // getDocument() returns the LIVE document; axios serializes it synchronously
    // before the await and `busy` guards re-entry, so it can't mutate mid-flight.
    const doc = editor.getDocument()
    try {
        if (currentId.value) {
            await drawings.update(currentId.value, doc)
            message.value = 'Saved.'
        } else {
            const meta = await drawings.create(doc)
            currentId.value = meta.id
            message.value = `Saved as ${meta.id}.`
        }
    } catch (err) {
        reportError(err, 'save')
    } finally {
        busy.value = false
    }
}

async function load() {
    if (!editor || busy.value) return
    busy.value = true
    message.value = null
    try {
        const page = await drawings.list({ limit: 1, kind: 'free' })
        const first = page.drawings[0]
        if (!first) {
            message.value = 'No saved drawings yet.'
            return
        }
        const full = await drawings.get(first.id)
        // full.document is already validated by drawings.get (parseDocument).
        editor.loadDocument(full.document)
        currentId.value = full.id
        message.value = `Loaded ${full.id}.`
    } catch (err) {
        reportError(err, 'load')
    } finally {
        busy.value = false
    }
}
</script>

<template>
    <div class="draw">
        <EditorToolbar
            :active-tool="ui.activeTool"
            :color="ui.color"
            :stroke-width="ui.strokeWidth"
            :fill-enabled="ui.fillEnabled"
            :fill="ui.fill"
            :busy="busy"
            :message="message"
            @pick-tool="pickTool"
            @set-color="setColor"
            @set-width="setWidth"
            @toggle-fill="toggleFill"
            @set-fill="setFill"
            @clear="clearCanvas"
            @export-png="exportPng"
            @save="save"
            @load="load"
        />

        <!-- The Editor sizes its Konva stage to doc.width x doc.height
             (1920x1080) with no fit-to-viewport, so the wrapper scrolls. -->
        <div class="draw__scroll">
            <div ref="containerRef" class="draw__canvas"></div>
        </div>
    </div>
</template>

<style scoped>
.draw {
    height: 100%;

    display: flex;
    flex-direction: column;
}

.draw__scroll {
    flex: 1 1 auto;
    min-height: 0;

    overflow: auto;

    display: flex;
    align-items: flex-start;
    justify-content: center;

    padding: 1rem;

    background-color: var(--ori-color-background);
}

.draw__canvas {
    flex: none;

    box-shadow: 0 0 0 1px var(--ori-color-outline, rgb(0 0 0 / 15%));
    background-color: #fff;
}
</style>
