<script setup lang="ts">
import { useCanvasStore } from '@shared/stores';
import { onBeforeMount, onMounted, ref } from 'vue';

const canvasStore = useCanvasStore();

const canvas = ref();

const showCanvas = ref(false);
const canvasWidth = ref(200);
const canvasHeight = ref(200);

onBeforeMount(() => {
    canvasWidth.value = +(document.body.clientWidth * 0.7).toFixed(0);
    canvasHeight.value = +(document.body.clientHeight * 0.7).toFixed(0);
    showCanvas.value = true;
});

onMounted(() => {
    canvasStore.setCanvas(canvas.value);
});
</script>

<template>
    <canvas
        v-if="showCanvas"
        ref="canvas"
        class="canvas"
        :width="canvasWidth + 'px'"
        :height="canvasHeight + 'px'"
        @mousedown=""
        @mousemove=""
        @mouseup=""
    ></canvas>
</template>

<style>
.canvas {
    background-color: white;
}
</style>
