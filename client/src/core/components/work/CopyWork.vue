<script setup lang="ts">
import { ref } from 'vue'
import { VButton } from 'vueinjar'
import { copyToClipboard, icons } from '@core'
import { getCanvasBlob, useCanvasStore } from '@modules/canvas'

async function handleCopyWork() {
    const canvas = useCanvasStore().canvas

    if (canvas) {
        const canvasBlob = await getCanvasBlob(canvas)
        await copyToClipboard('image', canvasBlob)
    }
}
</script>

<template>
    <v-button :icon="icons.work.copy" text="Copy" radius="md" variant="tonal" fluid @click="handleCopyWork" />
</template>
