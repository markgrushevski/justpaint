<script setup lang="ts">
import { ref } from 'vue'
import { VButton } from 'vueinjar'
import { copyToClipboard, icons } from '@core'
import { getCanvasBlob, getCanvasDataURL } from '@modules/canvas'

const p = defineProps<{ column?: boolean; canvas?: HTMLCanvasElement | null; dataUrl?: string }>()
const emit = defineEmits<{
    (e: 'copyArt', value: 'text' | 'art'): void
}>()

async function handleCopyTextArt() {
    emit('copyArt', 'text')
    if (p.canvas) {
        const dataURL = getCanvasDataURL(p.canvas)
        await copyToClipboard('text', dataURL)
    } else if (p.dataUrl) {
        await copyToClipboard('text', p.dataUrl)
    }
}

async function handleCopyArt() {
    emit('copyArt', 'art')
    if (p.canvas) {
        const blob = await getCanvasBlob(p.canvas)
        await copyToClipboard('image', blob)
    } else if (p.dataUrl) {
        const blob = await (await fetch(p.dataUrl)).blob()
        await copyToClipboard('image', blob)
    }
}
</script>

<template>
    <div :class="{ 'copy-handlers_column': column }" class="copy-handlers">
        <v-button
            :icon="icons.art.copy"
            :variant="column ? 'text' : 'tonal'"
            :size="column ? 'sm' : 'md'"
            :text="column ? 'text' : 'Copy as text'"
            :iconPosition="column ? 'right' : 'left'"
            radius="md"
            @click="handleCopyTextArt"
        />
        <v-button
            :icon="icons.art.copy"
            :variant="column ? 'text' : 'tonal'"
            :size="column ? 'sm' : 'md'"
            :text="column ? 'image' : 'Copy as image'"
            :iconPosition="column ? 'right' : 'left'"
            radius="md"
            @click="handleCopyArt"
        />
    </div>
</template>

<style>
.copy-handlers {
    display: flex;
    align-items: center;
    gap: var(--v-size-gap);
}

.copy-handlers.copy-handlers_column {
    flex-direction: column;
    gap: 0;
}

.copy-handlers.copy-handlers_column > * {
    flex-basis: auto;
    justify-content: normal;
    width: 100%;
}

.copy-handlers > * {
    flex-grow: 1;
    flex-basis: 30%;
}
</style>
