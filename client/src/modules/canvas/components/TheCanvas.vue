<script setup lang="ts">
import { onMounted, useTemplateRef, watch } from 'vue'
import { resizeCanvas } from '../utils'
import { useCanvasStore } from '../stores'

const canvasStore = useCanvasStore()

const canvasRef = useTemplateRef<HTMLCanvasElement>('canvas')

onMounted(() => {
    window.addEventListener('resize', async () => {
        if (canvasRef.value instanceof HTMLCanvasElement) {
            await resizeCanvas(canvasRef.value)
        }
    })
})

const stopWatch = watch(canvasRef, () => {
    if (canvasRef.value instanceof HTMLCanvasElement) {
        canvasStore.canvas = canvasRef.value
        resizeCanvas(canvasRef.value)
        stopWatch()
    }
})
</script>

<template>
    <canvas ref="canvas"></canvas>
</template>
