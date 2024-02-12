<script setup lang="ts">
import { useToolsStore } from '@shared/stores';
import PaintTool from './PaintTool.vue';
</script>

<template>
    <PaintTool type="pen" />
    <PaintTool type="eraser" />
</template>
