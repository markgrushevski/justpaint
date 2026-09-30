/**
 * Hosts one Editor in an EditorShell: mounts it, mirrors its state for the chrome,
 * wires the toolbar and the shared keyboard shortcuts, and destroys it on unmount.
 * Every canvas route (/draw, /play, /practice) uses it.
 */
import { computed, onBeforeUnmount, onMounted, onUnmounted, reactive, ref, shallowRef, watch } from 'vue'
import { useThemeColor } from '@oriui/headless/vue'
import { DEFAULT_CANVAS, DEFAULT_STYLE, Editor, TOOLS } from '@justpaint/editor'
import type { Document, LayerView, ToolId } from '@justpaint/editor'
import { useAuthGate } from '@core'
import { TOOL_META } from './FloatingToolbar.vue'

export interface EditorHostOptions {
    /** The document the editor opens with, built at mount when the canvas has its size. */
    initialDocument: (canvas: HTMLDivElement) => Document
    /** Ctrl/Cmd shortcuts beyond undo, redo and zoom, by lowercase key (`enter`, `s`). */
    commands?: Record<string, () => void>
    /** Runs on a plain key before the tool keys; returning true stops there. */
    beforeToolKeys?: (e: KeyboardEvent) => boolean
}

const KEY_TO_TOOL = new Map<string, ToolId>(
    (Object.keys(TOOLS) as ToolId[]).map((id) => [TOOL_META[id].key.toLowerCase(), id])
)

function isTextField(target: EventTarget | null): boolean {
    const el = target as HTMLElement | null
    return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable)
}

export function useEditorHost(options: EditorHostOptions) {
    /** Bound to `<EditorShell ref="shell">`, which exposes the Konva mount element. */
    const shell = ref<{ canvasEl: HTMLDivElement | null } | null>(null)
    const editor = shallowRef<Editor | null>(null)
    let unsubscribe: (() => void) | null = null

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
    // DEFAULT_CANVAS is `as const`; a bare ref() would narrow to the literal.
    const docWidth = ref<number>(DEFAULT_CANVAS.width)
    const docHeight = ref<number>(DEFAULT_CANVAS.height)
    const isEmpty = computed(() => layers.value.every((l) => l.strokeCount === 0))

    function sync() {
        const ed = editor.value
        if (!ed) return
        layers.value = ed.getLayers()
        activeLayerId.value = ed.getActiveLayerId()
        canUndo.value = ed.canUndo()
        canRedo.value = ed.canRedo()
        zoom.value = ed.getZoom()
        const doc = ed.getDocument()
        docWidth.value = doc.width
        docHeight.value = doc.height
    }

    // Konva can't read CSS variables, so the cursor ring gets the primary token resolved
    // on every theme flip.
    const cursorRingColor = useThemeColor('primary')
    watch(cursorRingColor, (color) => editor.value?.setCursorColor(color || null))

    function pickTool(id: ToolId) {
        ui.activeTool = id
        editor.value?.setTool(TOOLS[id]) // setTool takes the Tool object, not the id
    }
    function setColor(hex: string) {
        ui.color = hex
        editor.value?.setStyle({ color: hex })
    }
    function setWidth(width: number) {
        ui.strokeWidth = width
        editor.value?.setStyle({ strokeWidth: width })
    }
    function toggleFill(enabled: boolean) {
        ui.fillEnabled = enabled
        editor.value?.setStyle({ fill: enabled ? ui.fill : null })
    }
    function setFill(hex: string) {
        ui.fill = hex
        if (ui.fillEnabled) editor.value?.setStyle({ fill: hex })
    }
    function undo() {
        editor.value?.undo()
    }
    function redo() {
        editor.value?.redo()
    }
    function zoomIn() {
        editor.value?.zoomIn()
    }
    function zoomOut() {
        editor.value?.zoomOut()
    }
    function fitView() {
        editor.value?.fitToViewport()
    }

    /** Replace the document; this drops the undo history. */
    function load(doc: Document) {
        editor.value?.loadDocument(doc)
        sync()
    }

    /** The drawing as a PNG at its own size. */
    async function toPNG(): Promise<Blob | null> {
        const ed = editor.value
        if (!ed) return null
        const doc = ed.getDocument()
        return ed.toPNG({ outWidth: doc.width, outHeight: doc.height, fit: 'contain' })
    }

    const gate = useAuthGate()
    const editorKeys: Record<string, () => void> = {
        z: undo,
        y: redo,
        '0': fitView,
        '=': zoomIn,
        '+': zoomIn,
        '-': zoomOut
    }

    function onKeydown(e: KeyboardEvent) {
        // The sign-in modal owns the keyboard: a command behind it would act on the old session.
        if (gate.open) return
        if (isTextField(e.target)) return
        const key = e.key.toLowerCase()
        if (e.ctrlKey || e.metaKey) {
            const command = key === 'z' && e.shiftKey ? redo : (options.commands?.[key] ?? editorKeys[key])
            if (command) {
                e.preventDefault()
                command()
            }
            return
        }
        if (e.altKey) return
        if (options.beforeToolKeys?.(e)) return
        const tool = KEY_TO_TOOL.get(key)
        if (tool) {
            e.preventDefault()
            pickTool(tool)
        }
    }

    onMounted(() => {
        const canvas = shell.value?.canvasEl ?? null
        if (!canvas) return
        const ed = new Editor(canvas, options.initialDocument(canvas))
        ed.setTool(TOOLS[ui.activeTool])
        ed.setStyle({ ...DEFAULT_STYLE })
        // useThemeColor resolves in an earlier mounted hook; the watch covers late changes.
        ed.setCursorColor(cursorRingColor.value || null)
        editor.value = ed
        unsubscribe = ed.onChange(sync)
        sync()
        window.addEventListener('keydown', onKeydown)
    })

    onBeforeUnmount(() => {
        window.removeEventListener('keydown', onKeydown)
    })

    // After every onBeforeUnmount, so a view's own teardown can still reach the editor.
    // Dropping the ref alone would leak the stage: Konva keeps a module-global registry.
    onUnmounted(() => {
        unsubscribe?.()
        unsubscribe = null
        editor.value?.destroy()
        editor.value = null
    })

    return {
        shell,
        editor,
        ui,
        layers,
        activeLayerId,
        canUndo,
        canRedo,
        zoomPercent,
        docWidth,
        docHeight,
        isEmpty,
        pickTool,
        setColor,
        setWidth,
        toggleFill,
        setFill,
        undo,
        redo,
        zoomIn,
        zoomOut,
        fitView,
        load,
        toPNG
    }
}
