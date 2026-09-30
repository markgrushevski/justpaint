/** The pointer position in document coords, for the corner readout; null off-canvas. */
import { onBeforeUnmount, onMounted, ref, type ShallowRef } from 'vue'
import type { Editor } from '@justpaint/editor'

export function useCanvasCoords(editor: ShallowRef<Editor | null>, canvas: () => HTMLDivElement | null) {
    const coords = ref<{ x: number; y: number } | null>(null)
    let host: HTMLDivElement | null = null
    // One read per animation frame, not per pointermove.
    let raf = 0
    let lastPointer: { x: number; y: number } | null = null

    function onPointerMove(e: PointerEvent) {
        lastPointer = { x: e.clientX, y: e.clientY }
        if (raf) return
        raf = requestAnimationFrame(() => {
            raf = 0
            if (!editor.value || !lastPointer) return
            coords.value = editor.value.toDocumentCoords(lastPointer.x, lastPointer.y)
        })
    }

    function onPointerLeave() {
        if (raf) {
            cancelAnimationFrame(raf)
            raf = 0
        }
        lastPointer = null
        coords.value = null
    }

    onMounted(() => {
        host = canvas()
        host?.addEventListener('pointermove', onPointerMove)
        host?.addEventListener('pointerleave', onPointerLeave)
    })

    onBeforeUnmount(() => {
        if (raf) cancelAnimationFrame(raf)
        host?.removeEventListener('pointermove', onPointerMove)
        host?.removeEventListener('pointerleave', onPointerLeave)
        host = null
    })

    return { coords }
}
