/**
 * The view-only paper behind a drawing with no background of its own: white, or, where the
 * view allows it, a checkerboard. The editor keeps it out of exports and the judged raster.
 */
import { computed, onMounted, ref, watch, type ShallowRef } from 'vue'
import type { Editor } from '@justpaint/editor'
import { useThemeStore } from '@core'

const PREF_KEY = 'jp.backdropGrid'

/**
 * The paper every drawing sits on, the same white the judge renders a scored drawing on
 * (`JUDGE_BG` in packages/render). The gallery paints its previews on it too.
 */
export const PAPER = '#ffffff'

/**
 * 8px checkerboard tiles over the desk, dark squares for a light desk and light ones for a
 * dark desk; built lazily and shared across mounts.
 */
const GRID_TILES = {
    light: "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='12' y='0' width='12' height='12' fill='%230002'/%3e%3crect x='0' y='12' width='12' height='12' fill='%230002'/%3e%3c/svg%3e",
    dark: "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='0' y='0' width='12' height='12' fill='%23fff2'/%3e%3crect x='12' y='12' width='12' height='12' fill='%23fff2'/%3e%3c/svg%3e"
}
const gridTiles: { light: HTMLImageElement | null; dark: HTMLImageElement | null } = { light: null, dark: null }

function gridTile(key: 'light' | 'dark'): HTMLImageElement {
    let img = gridTiles[key]
    if (!img) {
        img = new Image()
        img.src = GRID_TILES[key]
        gridTiles[key] = img
    }
    return img
}

/** `judged` is for the scored modes: the sheet is what the judge sees, so no checkerboard. */
export function useBackdrop(editor: ShallowRef<Editor | null>, { judged = false } = {}) {
    const theme = useThemeStore()
    const grid = ref(false)
    // An inverted canvas turns the light tile's squares light itself (main.css).
    const tileKey = computed(() => (theme.isDark && !theme.canvasInverted ? 'dark' : 'light'))

    async function apply() {
        const ed = editor.value
        if (!ed) return
        if (!grid.value) {
            ed.setCanvasBackdrop({ type: 'color', color: PAPER })
            return
        }
        const img = gridTile(tileKey.value)
        if (img.complete) {
            ed.setCanvasBackdrop({ type: 'pattern', image: img })
            return
        }
        try {
            await img.decode()
        } catch {
            return // a data-URI that fails to decode won't succeed on retry
        }
        // The editor, the pref or the theme may have changed during the decode.
        if (editor.value !== ed || !grid.value || img !== gridTile(tileKey.value)) return
        ed.setCanvasBackdrop({ type: 'pattern', image: img })
    }

    watch([grid, tileKey], () => apply())

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
