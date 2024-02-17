<script setup lang="ts">
import { Pen, Eraser } from '@shared/lib/canvas';
import { useCanvasStore, useToolsStore } from '@shared/stores';
import { ToolbarIcon } from '@shared/ui';
import PaintTool from './PaintTool.vue';

const canvasStore = useCanvasStore();
const toolsStore = useToolsStore();
</script>

<template>
    <template v-if="canvasStore.canvas">
        <PaintTool :type="Pen.name" @click="toolsStore.setTool(new Pen(canvasStore.canvas))" />
        <PaintTool :type="Eraser.name" @click="toolsStore.setTool(new Eraser(canvasStore.canvas))" />
    </template>
</template>
