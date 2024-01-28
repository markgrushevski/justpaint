<script setup lang="ts">
import { useCanvasStore } from '@shared/stores';
import { onBeforeMount, onMounted, ref } from 'vue';

const canvasStore = useCanvasStore();

const _canvas = ref();

const canvasWidth = ref(200);
const canvasHeight = ref(200);
const showCanvas = ref(false);

onBeforeMount(() => {
    canvasWidth.value = +(document.body.clientWidth * 0.75).toFixed(0);
    canvasHeight.value = +(document.body.clientHeight * 0.75).toFixed(0);
    showCanvas.value = true;
});

onMounted(() => {
    canvasStore.setCanvas(_canvas.value);
});
</script>

<template>
    <canvas
        v-if="showCanvas"
        ref="_canvas"
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
