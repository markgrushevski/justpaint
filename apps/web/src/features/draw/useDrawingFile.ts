/**
 * The open drawing as a file: new, save, load, rename, export and copy. Save and load
 * need a session; everything else works signed out.
 */
import { computed, ref, watch, type Ref, type ShallowRef } from 'vue'
import { blankDocument, DEFAULT_CANVAS, LIMITS } from '@justpaint/editor'
import type { Document, Editor } from '@justpaint/editor'
import { copyImage, copyText, useLoadLatestDrawing, useSaveDrawing, useSessionStore } from '@core'
import type { useToast } from '@oriui/vue'
import { TOAST } from './useGatedActions'

const DEFAULT_NAME = 'new art'

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

    const saveMutation = useSaveDrawing()
    const loadMutation = useLoadLatestDrawing()
    const busy = computed(() => saveMutation.isPending.value || loadMutation.isPending.value)

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

    // Ungated: only save needs a session.
    function rename(next: string) {
        name.value = next.trim() || DEFAULT_NAME
    }

    async function save() {
        if (!editor.value || busy.value) return
        if (!(await deps.gated('Sign in to save your drawing.'))) return
        // Browser Back can unmount the view while the modal is up.
        const ed = editor.value
        if (!ed) return
        const existing = currentId.value
        saveMutation.mutate(
            { id: existing ?? undefined, document: ed.getDocument(), name: name.value },
            {
                onSuccess: (meta) => {
                    currentId.value = meta.id
                    toaster.success({ text: existing ? 'Saved.' : `Saved as ${meta.id}.`, duration: TOAST.success })
                },
                onError: (err) => deps.reportError(err, 'save')
            }
        )
    }

    async function loadLatest() {
        if (!editor.value || busy.value) return
        if (!(await deps.gated('Sign in to load your drawing.'))) return
        if (!editor.value) return
        loadMutation.mutate(undefined, {
            onSuccess: (full) => {
                if (!full) {
                    toaster.info({ text: 'No saved drawings yet.', duration: TOAST.info })
                    return
                }
                deps.onReplace()
                deps.load(full.document)
                currentId.value = full.id
                name.value = full.name
                toaster.success({ text: `Loaded ${full.id}.`, duration: TOAST.success })
            },
            onError: (err) => deps.reportError(err, 'load')
        })
    }

    async function exportPng() {
        const blob = await deps.toPNG()
        if (!blob) return
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `justpaint-${Date.now()}.png`
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
        busy,
        confirmOpen,
        requestNew,
        applyCanvasSize,
        confirmNew,
        cancelNew,
        rename,
        save,
        loadLatest,
        exportPng,
        copyJson,
        copyPng
    }
}
