<script setup lang="ts">
import { useCanvasStore } from '@shared/stores';
import { onBeforeMount, ref, watch } from 'vue';

const canvasStore = useCanvasStore();

const canvas = ref<HTMLCanvasElement | null>(null);

onBeforeMount(() => {
    canvasStore.canvasWidth = Math.round(document.body.clientWidth * 0.7);
    canvasStore.canvasHeight = Math.round(document.body.clientHeight * 0.7);
    canvasStore.showCanvas = true;
});

const stopWatch = watch(canvas, () => {
    if (canvas.value instanceof HTMLCanvasElement) {
        canvasStore.setCanvas(canvas.value);
        stopWatch();
    }
});
</script>

<template>
    <canvas
        v-if="canvasStore.showCanvas"
        ref="canvas"
        class="canvas"
        :width="canvasStore.canvasWidth + 'px'"
        :height="canvasStore.canvasHeight + 'px'"
    ></canvas>
</template>

<style>
.canvas {
    background-color: white;
    /*box-shadow: 0 0 5px 0 rgba(0, 0, 0, 0.1);*/
}
</style>
