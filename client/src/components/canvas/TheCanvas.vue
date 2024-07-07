<script setup lang="ts">
import { CanvasHistory } from '@shared/lib'
import { useCanvasHistoryStore, useCanvasStore } from '@shared/stores'
import { onBeforeMount, onMounted, ref, watch } from 'vue'

const canvasStore = useCanvasStore()
const historyStore = useCanvasHistoryStore()

const canvas = ref<HTMLCanvasElement | null>(null)
const showCanvas = ref(false)

onBeforeMount(() => {
    canvasStore.canvasWidth = Math.round(document.body.clientWidth * 0.7)
    canvasStore.canvasHeight = Math.round(document.body.clientHeight * 0.7)
})

onMounted(() => {
    showCanvas.value = true
})

const stopWatch = watch(canvas, () => {
    if (canvas.value instanceof HTMLCanvasElement) {
        canvasStore.setCanvas(canvas.value)
        historyStore.setHistoryHandler(new CanvasHistory(canvas.value))
        stopWatch()
    }
})
</script>

<template>
    <canvas
        v-if="showCanvas"
        ref="canvas"
        :width="canvasStore.canvasWidth + 'px'"
        :height="canvasStore.canvasHeight + 'px'"
    ></canvas>
</template>

<style>
canvas {
    background-color: white;
    box-shadow: 0 0 5px 0 rgba(0, 0, 0, 0.1);
}
</style>
