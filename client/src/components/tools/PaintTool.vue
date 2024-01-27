<script setup lang="ts">
import { useToolsStore } from '@shared/stores';
import type { Tool } from '@shared/types';
import { ToolbarIcon } from '@shared/ui';

defineProps<{
    type: Tool;
}>();

const toolsStore = useToolsStore();
</script>

<template>
    <ToolbarIcon
        class="toolbar__paint-tool"
        :type="type"
        :is-active="toolsStore.currentTool === type"
        @click="toolsStore.setTool(type)"
    />
</template>
