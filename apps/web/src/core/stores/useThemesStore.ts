import { defineStore } from 'pinia'
import { ref } from 'vue'

const themes = ['auto', 'light', 'dark'] as const
type Theme = (typeof themes)[number]

export const useThemesStore = defineStore('themes', () => {
    const currentTheme = ref<Theme>('auto')

    function setTheme(theme: string) {
        if (themes.includes(<Theme>theme)) {
            localStorage.setItem('theme', theme)
            currentTheme.value = <Theme>theme

            if (theme === 'auto') {
                document.documentElement.className = getUserThemeName()
            } else {
                document.documentElement.className = theme
            }
        } else {
            setTheme('auto')
        }
    }

    function toggleTheme() {
        if (currentTheme.value === 'auto') setTheme('light')
        else if (currentTheme.value === 'light') setTheme('dark')
        else setTheme('auto')
    }

    function getThemeName(): Exclude<Theme, 'auto'> {
        if (currentTheme.value === 'auto') return getUserThemeName()
        else return currentTheme.value
    }

    function getUserThemeName(): Exclude<Theme, 'auto'> {
        if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
            return 'dark'
        } else {
            return 'light'
        }
    }

    return { currentTheme, setTheme, toggleTheme, getThemeName, getUserThemeName }
})
