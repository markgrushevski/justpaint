import { defineStore } from 'pinia'
import { computed, ref, watchEffect } from 'vue'

/** The user-facing theme mode; `auto` follows the OS preference live. */
export type ThemeMode = 'auto' | 'light' | 'dark'

const STORAGE_KEY = 'jp-theme'

function storedMode(): ThemeMode {
    const v = localStorage.getItem(STORAGE_KEY)
    return v === 'light' || v === 'dark' ? v : 'auto'
}

/**
 * Light/dark theme (the legacy ThemeToggler pattern: cycle auto → light → dark,
 * persist, apply on <html>). ONE source of truth for "dark": the `ori-theme_dark`
 * class — oriui flips its own tokens on it, and main.css keys the justpaint brand
 * aliases off the same class. `auto` is implemented HERE via matchMedia (not a
 * CSS media query, which the in-app toggle could not override).
 */
export const useThemeStore = defineStore('theme', () => {
    const mode = ref<ThemeMode>(storedMode())

    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const systemDark = ref(media.matches)
    media.addEventListener('change', (e) => {
        systemDark.value = e.matches
    })

    const isDark = computed(() => mode.value === 'dark' || (mode.value === 'auto' && systemDark.value))

    watchEffect(() => {
        document.documentElement.classList.toggle('ori-theme_dark', isDark.value)
        localStorage.setItem(STORAGE_KEY, mode.value)
    })

    /** Cycle auto → light → dark → auto (the legacy toggle order). */
    function cycle(): void {
        mode.value = mode.value === 'auto' ? 'light' : mode.value === 'light' ? 'dark' : 'auto'
    }

    return { mode, isDark, cycle }
})
