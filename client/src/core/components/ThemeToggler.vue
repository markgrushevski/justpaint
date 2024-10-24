<script setup lang="ts">
import { VButton } from 'vueinjar'
import { computed, onMounted, ref } from 'vue'
import { mdiThemeLightDark, mdiWeatherNight, mdiWeatherSunny } from '@mdi/js'
import { useThemesStore } from '../stores'

const themesStore = useThemesStore()

const currentIcon = computed(() => {
    if (themesStore.currentTheme === 'light') return mdiWeatherSunny
    else if (themesStore.currentTheme === 'dark') return mdiWeatherNight
    else return mdiThemeLightDark
})

onMounted(() => {
    themesStore.setTheme(localStorage.getItem('theme') || 'auto')
})
</script>

<template>
    <v-button
        :icon="currentIcon"
        :title="themesStore.currentTheme"
        variant="text"
        size="lg"
        @click="themesStore.toggleTheme"
    />
</template>
