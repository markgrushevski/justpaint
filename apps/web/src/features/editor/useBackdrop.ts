/**
 * The view-only paper behind the drawing: white, or, where the view allows it, a
 * checkerboard. The editor keeps it out of exports and the judged raster. The dark theme
 * darkens a free drawing's paper through its ink view (main.css), not here.
 */
import { onMounted, ref, watch, type ShallowRef } from 'vue'
import type { Editor } from '@justpaint/editor'

const PREF_KEY = 'jp.backdropGrid'

/**
 * The paper every drawing sits on, the same white the judge renders a scored drawing on
 * (`JUDGE_BG` in packages/render). The gallery paints its previews on it too.
 */
export const PAPER = '#ffffff'

/** An 8px checkerboard tile, built lazily and shared across mounts. */
const GRID_TILE =
    "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='12' y='0' width='12' height='12' fill='%230002'/%3e%3crect x='0' y='12' width='12' height='12' fill='%230002'/%3e%3c/svg%3e"

let gridTileImage: HTMLImageElement | null = null

function gridTile(): HTMLImageElement {
    if (!gridTileImage) {
        gridTileImage = new Image()
        gridTileImage.src = GRID_TILE
    }
    return gridTileImage
}

/** `judged` is for the scored modes: the sheet is what the judge sees, so no checkerboard. */
export function useBackdrop(editor: ShallowRef<Editor | null>, { judged = false } = {}) {
    const grid = ref(false)

    async function apply() {
        const ed = editor.value
        if (!ed) return
        if (!grid.value) {
            ed.setCanvasBackdrop({ type: 'color', color: PAPER })
            return
        }
        const img = gridTile()
        if (img.complete) {
            ed.setCanvasBackdrop({ type: 'pattern', image: img })
            return
        }
        try {
            await img.decode()
        } catch {
            return // a data-URI that fails to decode won't succeed on retry
        }
        // The editor or the pref may have changed during the decode.
        if (editor.value !== ed || !grid.value) return
        ed.setCanvasBackdrop({ type: 'pattern', image: img })
    }

    watch(grid, () => apply())

    function setGrid(on: boolean) {
        grid.value = on
        try {
            localStorage.setItem(PREF_KEY, on ? '1' : '0')
        } catch {
            /* private mode / storage disabled — the pref just won't persist */
        }
    }

    // After the editor host's own onMounted, so the editor exists.
    onMounted(() => {
        if (!judged) {
            try {
                grid.value = localStorage.getItem(PREF_KEY) === '1'
            } catch {
                /* private mode / storage disabled — the paper backdrop */
            }
        }
        apply()
    })

    return { grid, setGrid }
}
