<script setup lang="ts">
import { ref } from 'vue'
import { VButton } from 'vueinjar'
import { copyToClipboard, icons } from '@core'
import { getCanvasBlob, getCanvasDataURL, useCanvasStore } from '@modules/canvas'

const p = defineProps<{ column?: boolean; canvas?: HTMLCanvasElement | null }>()

async function handleCopyTextArt() {
    if (p.canvas) {
        const canvasDataURL = getCanvasDataURL(p.canvas)
        await copyToClipboard('text', canvasDataURL)
    }
}

async function handleCopyArt() {
    if (p.canvas) {
        const canvasBlob = await getCanvasBlob(p.canvas)
        await copyToClipboard('image', canvasBlob)
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
