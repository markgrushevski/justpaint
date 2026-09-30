/**
 * A drawing's preview, rendered in the browser from its document: no thumbnail is stored.
 * It repaints when the theme flips, since the paper behind the strokes changes with it.
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { renderToPNG } from '@justpaint/editor'
import { useDrawing, useThemeStore } from '@core'
import { paperColor } from '../editor/useBackdrop'

export const THUMB_WIDTH = 400
export const THUMB_HEIGHT = 300

export function useThumbnail(id: () => string) {
    const theme = useThemeStore()
    const { data, isError } = useDrawing(id)

    const src = ref<string | null>(null)
    const renderFailed = ref(false)

    function setSrc(next: string | null): void {
        if (src.value) URL.revokeObjectURL(src.value)
        src.value = next
    }

    // The previous image stays up until the next one is ready, so a theme flip never
    // flashes the skeleton. `stale` drops a render that a newer one overtook.
    watch(
        [() => data.value?.document, () => theme.isDark],
        async ([doc, dark], _previous, onCleanup) => {
            if (!doc) return
            let stale = false
            onCleanup(() => {
                stale = true
            })
            try {
                const blob = await renderToPNG(doc, {
                    outWidth: THUMB_WIDTH,
                    outHeight: THUMB_HEIGHT,
                    fit: 'contain',
                    // A document with its own background keeps it; the rest sit on the editor's paper.
                    background: doc.background ?? paperColor(dark)
                })
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
        failed: computed(() => !src.value && (renderFailed.value || isError.value))
    }
}
