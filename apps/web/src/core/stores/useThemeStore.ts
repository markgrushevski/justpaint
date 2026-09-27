import { useTheme } from '@oriui/headless/vue'
import { defineStore } from 'pinia'
import { computed } from 'vue'

/** The user-facing theme mode; `auto` follows the OS preference live. */
export type ThemeMode = 'auto' | 'light' | 'dark'

/** `localStorage` key the setting is persisted under (kept for the toggler + docs/main.css). */
const STORAGE_KEY = 'jp-theme'

/**
 * Light/dark theme, delegated to oriui's headless `useTheme` (a thin Vue
 * wrapper over `createThemeController`): it owns the `auto` matchMedia
 * plumbing, persistence under `STORAGE_KEY`, and applying the
 * `ori-theme_dark` class via `applyTheme` — the one source of truth for
 * "dark" that both oriui and main.css key off. `applyTheme` also works around
 * a Chromium style-invalidation bug where components otherwise keep the
 * previous theme's colours after a runtime toggle (oriui `theme.ts`,
 * `flushThemeInvalidation`). This store just wraps that controller as Pinia
 * state.
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

    return { mode, isDark, cycle }
})
