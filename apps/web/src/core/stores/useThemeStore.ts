import { flushThemeInvalidation } from '@oriui/headless'
import { useTheme } from '@oriui/headless/vue'
import { defineStore } from 'pinia'
import { computed, ref, watchEffect } from 'vue'

/** The user-facing theme mode; `auto` follows the OS preference live. */
export type ThemeMode = 'auto' | 'light' | 'dark'

/** The accents `main.css` defines; orange is the brand default. */
export const ACCENTS = ['orange', 'green', 'blue', 'violet'] as const
export type Accent = (typeof ACCENTS)[number]

/** How the free-drawing canvas looks: with the theme, or light or dark regardless. */
export type CanvasMode = 'auto' | 'light' | 'dark'

/** `localStorage` key the setting is persisted under (kept for the toggler + docs/main.css). */
const STORAGE_KEY = 'jp-theme'
const ACCENT_KEY = 'jp-accent'
const CANVAS_KEY = 'jp-canvas'

function stored(key: string): string | null {
    try {
        return localStorage.getItem(key)
    } catch {
        return null
    }
}

function store(key: string, value: string): void {
    try {
        localStorage.setItem(key, value)
    } catch {
        /* private mode / storage disabled — the setting just won't persist */
    }
}

/**
 * A class on the root, not a data attribute: oriui's token observer (`useThemeColor`, the
 * canvas cursor ring) watches the root's `class` and `style` only. The flush is the same
 * Chromium workaround `applyTheme` runs after a theme flip.
 */
function applyAccent(accent: Accent): void {
    const root = document.documentElement
    for (const a of ACCENTS) root.classList.toggle(`jp-accent-${a}`, a === accent && a !== 'orange')
    flushThemeInvalidation(document.body)
}

/**
 * Light/dark theme, delegated to oriui's headless `useTheme` (a thin Vue
 * wrapper over `createThemeController`): it owns the `auto` matchMedia
 * plumbing, persistence under `STORAGE_KEY`, and applying the
 * `ori-theme_dark` class via `applyTheme` — the one source of truth for
 * "dark" that both oriui and main.css key off. `applyTheme` also works around
 * a Chromium style-invalidation bug where components otherwise keep the
 * previous theme's colours after a runtime toggle (oriui `theme.ts`,
 * `flushThemeInvalidation`). This store wraps that controller as Pinia state
 * and also holds the accent the player picks (orange unless changed) and how
 * the free-drawing canvas looks.
 */
export const useThemeStore = defineStore('theme', () => {
    const { theme, resolvedTheme, cycleTheme, setTheme } = useTheme({
        storageKey: STORAGE_KEY,
        default: 'auto'
    })

    /**
     * The current setting (`auto` → follow the OS live, or a pinned `light` / `dark`).
     * Writable: a direct assignment routes through the controller (apply + persist).
     */
    const mode = computed<ThemeMode>({
        get: () => theme.value,
        set: (next) => setTheme(next)
    })

    /** True when the resolved theme on the DOM is dark (tracks the OS scheme in `auto`). */
    const isDark = computed(() => resolvedTheme.value === 'dark')

    /** Cycle auto → light → dark → auto. */
    function cycle(): void {
        cycleTheme()
    }

    const accent = ref<Accent>(ACCENTS.find((a) => a === stored(ACCENT_KEY)) ?? 'orange')
    applyAccent(accent.value)

    function setAccent(next: Accent): void {
        accent.value = next
        applyAccent(next)
        store(ACCENT_KEY, next)
    }

    const canvas = ref<CanvasMode>((['light', 'dark'] as const).find((m) => m === stored(CANVAS_KEY)) ?? 'auto')
    const canvasDark = computed(() => (canvas.value === 'auto' ? isDark.value : canvas.value === 'dark'))
    // main.css's ink view keys off this class (see there).
    watchEffect(() => document.documentElement.classList.toggle('jp-canvas-dark', canvasDark.value))

    function setCanvas(next: CanvasMode): void {
        canvas.value = next
        store(CANVAS_KEY, next)
    }

    return { mode, isDark, cycle, accent, setAccent, canvas, canvasDark, setCanvas }
})
