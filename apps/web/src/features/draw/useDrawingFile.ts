/**
 * The open drawing as a file: new, save, open, export and copy. The first save asks for
 * a name; later saves keep it. Save and open need a session; everything else works
 * signed out.
 */
import { computed, ref, watch, type Ref, type ShallowRef } from 'vue'
import { blankDocument, DEFAULT_CANVAS, LIMITS } from '@justpaint/editor'
import type { Document, Editor } from '@justpaint/editor'
import { copyImage, copyText, drawings, useSaveDrawing, useSessionStore } from '@core'
import type { useToast } from '@oriui/vue'
import { TOAST } from './useGatedActions'

const DEFAULT_NAME = 'Untitled drawing'

function clampDim(n: number): number {
    return Math.min(LIMITS.maxCanvasDimension, Math.max(1, Math.round(n)))
}

/** A blank document; unsized, it fits the canvas element. */
export function fittedDocument(canvas: HTMLElement | null, w?: number, h?: number): Document {
    const width = clampDim(w ?? (canvas && canvas.clientWidth > 0 ? canvas.clientWidth : DEFAULT_CANVAS.width))
    const height = clampDim(h ?? (canvas && canvas.clientHeight > 0 ? canvas.clientHeight : DEFAULT_CANVAS.height))
    return blankDocument(width, height)
}

export interface DrawingFileDeps {
    editor: ShallowRef<Editor | null>
    canvas: () => HTMLDivElement | null
    isEmpty: Ref<boolean>
    load: (doc: Document) => void
    toPNG: () => Promise<Blob | null>
    gated: (reason: string) => Promise<boolean>
    reportError: (err: unknown, action: string) => void
    toaster: ReturnType<typeof useToast>
    /** The document is about to be replaced: drop whatever describes the old one. */
    onReplace: () => void
}

export function useDrawingFile(deps: DrawingFileDeps) {
    const { editor, toaster } = deps
    const currentId = ref<string | null>(null)
    const name = ref(DEFAULT_NAME)
    /** The name once the drawing is saved; null while it has never been. */
    const savedName = computed(() => (currentId.value ? name.value : null))

    const saveMutation = useSaveDrawing()
    const opening = ref(false)
    const busy = computed(() => saveMutation.isPending.value || opening.value)

    // The document as last saved or opened, to tell whether leaving would lose work.
    // Compared only when asked, so drawing never pays for it.
    let snapshot: string | null = null

    function isDirty(): boolean {
        if (deps.isEmpty.value || !editor.value) return false
        return JSON.stringify(editor.value.getDocument()) !== snapshot
    }

    // Another account signed in: the open drawing is the previous one's, and saving it
    // would 404 on the ownership-scoped PUT.
    const session = useSessionStore()
    watch(
        () => session.user?.id,
        (now, before) => {
            if (before && now && now !== before) currentId.value = null
        }
    )

    function clear(w?: number, h?: number) {
        if (!editor.value) return
        deps.onReplace()
        deps.load(fittedDocument(deps.canvas(), w, h))
        currentId.value = null
        name.value = DEFAULT_NAME
        snapshot = null
    }

    // Clearing drops history, so confirm only when there is work to lose.
    const confirmOpen = ref(false)
    const pendingSize = ref<{ w: number; h: number } | null>(null)

    function requestNew() {
        pendingSize.value = null
        if (deps.isEmpty.value) clear()
        else confirmOpen.value = true
    }

    function applyCanvasSize(w: number, h: number) {
        if (deps.isEmpty.value) {
            clear(w, h)
        } else {
            pendingSize.value = { w, h }
            confirmOpen.value = true
        }
    }

    function confirmNew() {
        const size = pendingSize.value
        pendingSize.value = null
        clear(size?.w, size?.h)
        confirmOpen.value = false
    }

    function cancelNew() {
        pendingSize.value = null
        confirmOpen.value = false
    }

    // Only a first save asks for a name.
    const nameOpen = ref(false)

    async function save() {
        if (!editor.value || busy.value) return
        if (!(await deps.gated('Sign in to save your drawing.'))) return
        if (currentId.value) write()
        else nameOpen.value = true
    }

    function confirmName(next: string) {
        nameOpen.value = false
        write(next)
    }

    function cancelName() {
        nameOpen.value = false
    }

    /** Creates with `newName` when there is no id yet; an update keeps the stored name. */
    function write(newName?: string) {
        // Browser Back can unmount the view while a modal is up.
        const ed = editor.value
        if (!ed) return
        const existing = currentId.value
        const document = ed.getDocument()
        const sent = JSON.stringify(document)
        saveMutation.mutate(
            { id: existing ?? undefined, document, name: existing ? undefined : newName },
            {
                onSuccess: (meta) => {
                    currentId.value = meta.id
                    name.value = meta.name
                    snapshot = sent
                    toaster.success({ text: `Saved “${meta.name}”.`, duration: TOAST.success })
                },
                onError: (err) => deps.reportError(err, 'save')
            }
        )
    }

    /** Opens a saved drawing; the gallery links to /draw with its id. */
    async function open(id: string) {
        if (busy.value) return
        if (!(await deps.gated('Sign in to open your drawing.'))) return
        opening.value = true
        try {
            const full = await drawings.get(id)
            if (!editor.value) return
            deps.onReplace()
            deps.load(full.document)
            currentId.value = full.id
            name.value = full.name
            snapshot = JSON.stringify(editor.value.getDocument())
        } catch (err) {
            deps.reportError(err, 'open')
        } finally {
            opening.value = false
        }
    }

    /** The saved name, stripped of characters a file name can't hold, or a timestamped default. */
    function fileName(): string {
        const safe = savedName.value?.replace(/[\\/:*?"<>|]+/g, '').trim()
        return safe || `justpaint-${Date.now()}`
    }

    async function exportPng() {
        const blob = await deps.toPNG()
        if (!blob) return
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `${fileName()}.png`
        a.click()
        // Revoking in the same tick as click() can abort the download in some browsers.
        setTimeout(() => URL.revokeObjectURL(url), 0)
    }

    async function copyJson() {
        if (!editor.value) return
        try {
            await copyText(JSON.stringify(editor.value.getDocument()))
            toaster.success({ text: 'Copied document JSON', duration: TOAST.success })
        } catch (err) {
            toaster.error({
                text: err instanceof Error ? err.message : 'Could not copy to the clipboard.',
                duration: TOAST.error
            })
        }
    }

    async function copyPng() {
        try {
            const blob = await deps.toPNG()
            if (!blob) return
            await copyImage(blob)
            toaster.success({ text: 'Copied image', duration: TOAST.success })
        } catch (err) {
            toaster.error({
                text: err instanceof Error ? err.message : 'Could not copy the image.',
                duration: TOAST.error
            })
        }
    }

    return {
        name,
        savedName,
        busy,
        isDirty,
        confirmOpen,
        requestNew,
        applyCanvasSize,
        confirmNew,
        cancelNew,
        nameOpen,
        save,
        confirmName,
        cancelName,
        open,
        exportPng,
        copyJson,
        copyPng
    }
}
