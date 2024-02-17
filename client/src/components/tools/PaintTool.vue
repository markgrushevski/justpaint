<script setup lang="ts">
import { useToolsStore } from '@shared/stores';
import type { ToolName } from '@shared/types';
import { ToolbarIcon } from '@shared/ui';

defineProps<{
    type: ToolName;
}>();

const toolsStore = useToolsStore();
</script>

<template>
    <ToolbarIcon class="toolbar__paint-tool" :type="type" :is-active="type === toolsStore.tool?.constructor.name" />
</template>
