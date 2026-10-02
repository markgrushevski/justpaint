import { flushThemeInvalidation } from '@oriui/headless'
import { useTheme } from '@oriui/headless/vue'
import { defineStore } from 'pinia'
import { computed, ref, watchEffect } from 'vue'
import { accentSources } from '../utils/color'

/** The user-facing theme mode; `auto` follows the OS preference live. */
export type ThemeMode = 'auto' | 'light' | 'dark'

/** The accents `main.css` defines; orange is the brand default. `custom` is a colour the player picked. */
export const ACCENTS = ['orange', 'green', 'blue', 'violet', 'custom'] as const
export type Accent = (typeof ACCENTS)[number]

/** `localStorage` key the setting is persisted under (kept for the toggler + docs/main.css). */
const STORAGE_KEY = 'jp-theme'
const ACCENT_KEY = 'jp-accent'
const CUSTOM_ACCENT_KEY = 'jp-accent-custom'
const INVERT_KEY = 'jp-canvas-invert'

const SOURCES = ['primary-light', 'on-primary-light', 'primary-dark', 'on-primary-dark'] as const

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
 * A preset is a class on the root and a picked colour is inline style there, never a data
 * attribute: oriui's token observer (`useThemeColor`, the canvas cursor ring) watches the
 * root's `class` and `style` only. The flush is the same Chromium workaround `applyTheme`
 * runs after a theme flip.
 */
function applyAccent(accent: Accent, custom: string): void {
    const root = document.documentElement
    for (const a of ACCENTS) root.classList.toggle(`jp-accent-${a}`, a === accent && a !== 'orange' && a !== 'custom')
    for (const name of SOURCES) root.style.removeProperty(`--ori-color-${name}`)
    if (accent === 'custom') {
        // The fill is measured against the pages main.css sets, so read them rather than copy them.
        const css = getComputedStyle(root)
        const page = (theme: 'light' | 'dark') => css.getPropertyValue(`--ori-color-background-${theme}`).trim()
        const s = accentSources(custom, page('light'), page('dark'))
        root.style.setProperty('--ori-color-primary-light', s.primaryLight)
        root.style.setProperty('--ori-color-on-primary-light', s.onPrimaryLight)
        root.style.setProperty('--ori-color-primary-dark', s.primaryDark)
        root.style.setProperty('--ori-color-on-primary-dark', s.onPrimaryDark)
    }
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
 * and also holds the accent the player picks (orange unless changed) and
 * whether the dark theme inverts the free-drawing canvas.
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
    const customAccent = ref(stored(CUSTOM_ACCENT_KEY) ?? '#e0218a')
    applyAccent(accent.value, customAccent.value)

    /** A preset, or `custom` with the colour to use. */
    function setAccent(next: Accent, color?: string): void {
        accent.value = next
        if (next === 'custom' && color) {
            customAccent.value = color
            store(CUSTOM_ACCENT_KEY, color)
        }
        applyAccent(next, customAccent.value)
        store(ACCENT_KEY, next)
    }

    /**
     * Excalidraw's dark canvas: in the dark theme a free drawing is shown inverted, so a
     * drawing made for light paper still reads. Off, the canvas shows its colours as they are.
     */
    const invertCanvas = ref(stored(INVERT_KEY) === '1')
    const canvasInverted = computed(() => invertCanvas.value && isDark.value)
    // main.css's ink view keys off this class (see there).
    watchEffect(() => document.documentElement.classList.toggle('jp-canvas-dark', canvasInverted.value))

    function setInvertCanvas(on: boolean): void {
        invertCanvas.value = on
        store(INVERT_KEY, on ? '1' : '0')
    }

    return {
        mode,
        isDark,
        cycle,
        accent,
        customAccent,
        setAccent,
        invertCanvas,
        canvasInverted,
        setInvertCanvas
    }
})
