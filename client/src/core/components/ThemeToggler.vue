<script setup lang="ts">
import { VButton } from 'vueinjar'
import { computed, onMounted, ref } from 'vue'
import { mdiThemeLightDark, mdiWeatherNight, mdiWeatherSunny } from '@mdi/js'

const themes = ['auto', 'light', 'dark'] as const
type Theme = (typeof themes)[number]

const currentTheme = ref<Theme>('auto')

const currentIcon = computed(() => {
    if (currentTheme.value === 'light') return mdiWeatherSunny
    else if (currentTheme.value === 'dark') return mdiWeatherNight
    else return mdiThemeLightDark
})

function setTheme(theme: string) {
    if (themes.includes(<Theme>theme)) {
        localStorage.setItem('theme', theme)
        currentTheme.value = <Theme>theme

        if (theme === 'auto') {
            if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
                document.documentElement.className = 'dark'
            } else {
                document.documentElement.className = 'light'
            }
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

onMounted(() => {
    setTheme(localStorage.getItem('theme') || 'auto')
})
</script>

<template>
    <v-button :icon="currentIcon" :title="currentTheme" variant="text" size="lg" @click="toggleTheme" />
</template>
