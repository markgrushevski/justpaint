<script setup lang="ts">
import { nextTick, onMounted, useTemplateRef, watch } from 'vue'
import { resizeCanvas } from '../utils'
import { useCanvasHistoryStore, useCanvasStore } from '../stores'
import { CanvasHistory } from '../models'

const canvasStore = useCanvasStore()
const historyStore = useCanvasHistoryStore()

const canvasRef = useTemplateRef<HTMLCanvasElement>('canvas')

onMounted(() => {
    /*window.addEventListener('resize', async () => {
        if (canvasRef.value instanceof HTMLCanvasElement) {
            await resizeCanvas(canvasRef.value)
        }
    })*/
})

const stopWatch = watch(canvasRef, () => {
    const canvas = canvasRef.value
    if (canvas instanceof HTMLCanvasElement) {
        stopWatch()
        resizeCanvas(canvas).then(() => {
            historyStore.setHistoryHandler(new CanvasHistory(canvas))
            canvasStore.canvas = canvas
        })
    }
})
</script>

<template>
    <canvas ref="canvas"></canvas>
</template>

<style>
canvas {
    background-repeat: repeat;
    background-image: url(../assets/canvas-grid-light.svg);
}

:root.dark canvas {
    background-image: url(../assets/canvas-grid-dark.svg);
}
</style>
