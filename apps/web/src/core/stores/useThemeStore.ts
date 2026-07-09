import { useTheme } from '@oriui/headless/vue'
import { defineStore } from 'pinia'
import { computed } from 'vue'

/** The user-facing theme mode; `auto` follows the OS preference live. */
export type ThemeMode = 'auto' | 'light' | 'dark'

/** `localStorage` key the setting is persisted under (kept for the toggler + docs/main.css). */
const STORAGE_KEY = 'jp-theme'

/**
 * Light/dark theme (the legacy ThemeToggler pattern: cycle auto → light → dark,
 * persist, apply on <html>). ONE source of truth for "dark": the `ori-theme_dark`
 * class — oriui flips its own tokens on it, and main.css keys the justpaint brand
 * aliases off the same class.
 *
 * The whole state machine is delegated to oriui's headless `useTheme` (a thin Vue
 * wrapper over `createThemeController`): it owns the `auto` matchMedia plumbing, the
 * persistence under `STORAGE_KEY`, and applying the class via `applyTheme` — which
 * flips `ori-theme_{light,dark}` on <html> AND works around a Chromium
 * style-invalidation bug where styled components otherwise keep the PREVIOUS theme's
 * colours after a runtime toggle (see oriui `theme.ts` / `flushThemeInvalidation`).
 * The controller applies the persisted/default theme immediately on construction
 * (no post-mount flash) and tears its OS-scheme listener down on store dispose.
 *
 * This store is now just the reactive Pinia projection of that controller: it keeps
 * the exact `{ mode, isDark, cycle }` shape the hand-rolled store exposed, so existing
 * callers (DrawView) are unchanged.
 */
export const useThemeStore = defineStore('theme', () => {
    const { theme, resolvedTheme, cycleTheme, setTheme } = useTheme({
        storageKey: STORAGE_KEY,
        default: 'auto'
    })

    /**
     * The current SETTING (`auto` → follow the OS live, or a pinned `light` / `dark`).
     * Writable: a direct assignment routes through the controller (apply + persist),
     * matching the old writable `mode` ref whose `watchEffect` applied on every change.
     */
    const mode = computed<ThemeMode>({
        get: () => theme.value,
        set: (next) => setTheme(next)
    })

    /** True when the RESOLVED theme on the DOM is dark (tracks the OS scheme in `auto`). */
    const isDark = computed(() => resolvedTheme.value === 'dark')

    /** Cycle auto → light → dark → auto (the legacy toggle order). */
    function cycle(): void {
        cycleTheme()
    }

    return { mode, isDark, cycle }
})
