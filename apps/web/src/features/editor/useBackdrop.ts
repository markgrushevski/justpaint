/**
 * The view-only paper behind the drawing: a warm paper color per theme, or, where the
 * view allows it, a checkerboard. The editor keeps it out of exports and the judged raster.
 */
import { onMounted, ref, watch, type ShallowRef } from 'vue'
import type { Editor } from '@justpaint/editor'
import { useThemeStore } from '@core'

const PREF_KEY = 'jp.backdropGrid'

/** The paper a drawing sits on; the gallery paints its previews on the same color. */
export function paperColor(dark: boolean): string {
    return dark ? '#12110f' : '#fdfcf8'
}

/** What the judge renders a scored drawing on (`JUDGE_BG` in packages/render), in either theme. */
const JUDGED_PAPER = '#ffffff'

/** 8px checkerboard tiles per theme; the images are built lazily and shared across mounts. */
const GRID_TILE_LIGHT =
    "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='12' y='0' width='12' height='12' fill='%230002'/%3e%3crect x='0' y='12' width='12' height='12' fill='%230002'/%3e%3c/svg%3e"
const GRID_TILE_DARK =
    "data:image/svg+xml,%3csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' width='8' height='8'%3e%3crect x='0' y='0' width='12' height='12' fill='%23fff2'/%3e%3crect x='12' y='12' width='12' height='12' fill='%23fff2'/%3e%3c/svg%3e"

const gridTiles: { light: HTMLImageElement | null; dark: HTMLImageElement | null } = { light: null, dark: null }

function gridTile(dark: boolean): HTMLImageElement {
    const key = dark ? 'dark' : 'light'
    let img = gridTiles[key]
    if (!img) {
        img = new Image()
        img.src = dark ? GRID_TILE_DARK : GRID_TILE_LIGHT
        gridTiles[key] = img
    }
    return img
}

/**
 * `judged` is for the scored modes: the sheet is what the judge sees, white in both
 * themes, so ink that vanishes for the judge vanishes on screen too; no checkerboard.
 */
export function useBackdrop(editor: ShallowRef<Editor | null>, { judged = false } = {}) {
    const theme = useThemeStore()
    const grid = ref(false)

    async function apply() {
        const ed = editor.value
        if (!ed) return
        if (!grid.value) {
            ed.setCanvasBackdrop({ type: 'color', color: judged ? JUDGED_PAPER : paperColor(theme.isDark) })
            return
        }
        const img = gridTile(theme.isDark)
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
        if (editor.value !== ed || !grid.value || img !== gridTile(theme.isDark)) return
        ed.setCanvasBackdrop({ type: 'pattern', image: img })
    }

    watch([() => theme.isDark, grid], () => apply())

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
