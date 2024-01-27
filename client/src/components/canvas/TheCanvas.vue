<script setup lang="ts">
import { useCanvasStore } from '@shared/stores';
import { onMounted, ref } from 'vue';

const canvasStore = useCanvasStore();

const _canvas = ref();

onMounted(() => {
    canvasStore.setCanvas(_canvas.value);
});
</script>

<template>
    <canvas ref="_canvas" class="canvas" width="800px" height="600px" @mousedown="" @mousemove="" @mouseup=""></canvas>
</template>

<style>
.canvas {
    background-color: white;
}
</style>
