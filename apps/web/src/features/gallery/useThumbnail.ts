/**
 * A drawing's preview, rendered in the browser from its document: no thumbnail is stored. The PNG is
 * transparent and cropped to the drawn content; the card paints the paper (`paper`) behind it, so the
 * image does not depend on the theme.
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { renderToStage } from '@justpaint/editor'
import type { Document } from '@justpaint/editor'
import { useDrawing, useThemeStore } from '@core'
import { paperColor } from '../editor/useBackdrop'
import { THUMB_WIDTH, thumbnailCrop } from './thumbnailCrop'
import type { Rect } from './thumbnailCrop'

type Stage = ReturnType<typeof renderToStage>

/** The box around everything visible on `stage`, or null when it is blank. */
function contentBounds(stage: Stage): Rect | null {
    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity

    for (const layer of stage.getLayers()) {
        if (!layer.visible() || layer.opacity() === 0) continue
        for (const node of layer.getChildren()) {
            // An eraser stroke only removes ink, so it never widens what can be seen.
            if (!node.visible() || node.globalCompositeOperation() === 'destination-out') continue
            const rect = node.getClientRect()
            if (rect.width === 0 || rect.height === 0) continue
            minX = Math.min(minX, rect.x)
            minY = Math.min(minY, rect.y)
            maxX = Math.max(maxX, rect.x + rect.width)
            maxY = Math.max(maxY, rect.y + rect.height)
        }
    }

    return minX === Infinity ? null : { x: minX, y: minY, width: maxX - minX, height: maxY - minY }
}

async function renderThumbnail(doc: Document): Promise<Blob> {
    // At scale 1 the stage's coordinates are the document's, so the bounds feed the crop as they are.
    const stage = renderToStage(doc, { outWidth: doc.width, outHeight: doc.height, background: null })
    try {
        const crop = thumbnailCrop(contentBounds(stage), doc.width, doc.height)
        const blob = await stage.toBlob({ ...crop, pixelRatio: THUMB_WIDTH / crop.width, mimeType: 'image/png' })
        if (!(blob instanceof Blob)) throw new Error('The preview could not be encoded.')
        return blob
    } finally {
        // Konva keeps every stage in a module-global registry until it is destroyed.
        stage.destroy()
    }
}

export function useThumbnail(id: () => string) {
    const theme = useThemeStore()
    const { data, isError } = useDrawing(id)

    const src = ref<string | null>(null)
    const renderFailed = ref(false)

    /** The paper behind the transparent preview: the document's own, else the editor's. */
    const paper = computed(() => data.value?.document.background ?? paperColor(theme.isDark))

    function setSrc(next: string | null): void {
        if (src.value) URL.revokeObjectURL(src.value)
        src.value = next
    }

    // The previous image stays up until the next one is ready, so a reload never flashes the
    // skeleton. `stale` drops a render that a newer one overtook.
    watch(
        () => data.value?.document,
        async (doc, _previous, onCleanup) => {
            if (!doc) return
            let stale = false
            onCleanup(() => {
                stale = true
            })
            try {
                const blob = await renderThumbnail(doc)
                if (stale) return
                setSrc(URL.createObjectURL(blob))
                renderFailed.value = false
            } catch {
                if (!stale) renderFailed.value = true
            }
        },
        { immediate: true }
    )

    onBeforeUnmount(() => setSrc(null))

    return {
        src,
        paper,
        failed: computed(() => !src.value && (renderFailed.value || isError.value))
    }
}
