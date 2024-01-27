<script setup lang="ts">
import { useToolsStore } from '@shared/stores';
import PaintTool from './PaintTool.vue';
</script>

<template>
    <div class="toolbar__paint-tools">
        <PaintTool type="pen" />
        <PaintTool type="eraser" />
    </div>
</template>

<style>
.toolbar__paint-tools {
    display: flex;
    align-items: center;
    gap: 4px;
}
</style>
