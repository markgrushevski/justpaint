<script lang="ts" setup>
import { onMounted, useTemplateRef, watch } from 'vue'
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
    <canvas ref="canvas" class="grid-bg"></canvas>
</template>

<style>
canvas {
    transition:
        0.15s filter ease-in-out,
        0.15s opacity ease-in-out;
}

canvas.blocked {
    pointer-events: none;
}
</style>
