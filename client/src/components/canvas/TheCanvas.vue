<script setup lang="ts">
import { injectionKeys } from '@shared/constants'
import { CanvasHistory, resizeCanvas } from '@shared/lib'
import { useCanvasHistoryStore, useCanvasStore } from '@shared/stores'
import { onBeforeMount, onMounted, provide, ref, useTemplateRef, watch } from 'vue'

const canvasStore = useCanvasStore()
const historyStore = useCanvasHistoryStore()

const canvasRef = useTemplateRef<HTMLCanvasElement>('canvas')

onMounted(async () => {
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
        historyStore.setHistoryHandler(new CanvasHistory(canvasRef.value))
        stopWatch()
    }
})
</script>

<template>
    <canvas ref="canvas"></canvas>
</template>

<style>
canvas {
    background-color: white;
    box-shadow: 0 0 5px 0 rgba(0, 0, 0, 0.1);
}
</style>
