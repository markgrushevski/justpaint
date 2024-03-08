<script setup lang="ts">
import { CanvasTool } from '@shared/lib';
import { useToolsStore } from '@shared/stores';
import { computed } from 'vue';

const toolsStore = useToolsStore();

const strokeColor = computed({
    get() {
        return toolsStore.tool?.strokeColor ?? '#000000';
    },
    set(value) {
        toolsStore.setColor(value);
    }
});

const fillColor = computed({
    get() {
        return toolsStore.tool?.fillColor ?? '#000000';
    },
    set(value) {
        toolsStore.setColor(value);
    }
});

const lineWeight = computed({
    get() {
        return toolsStore.tool?.lineWeight ?? 1;
    },
    set(value) {
        toolsStore.setLineWeight(value);
    }
});

const minWidth = computed(() => `${lineWeight.value}`.length + 4 + 'ch');
</script>

<template>
    <input v-model.lazy="fillColor" title="Fill color" type="color" />
    <input v-model.lazy="strokeColor" title="Stroke color" type="color" />
    <div title="Line weight"><input v-model.lazy.trim="lineWeight" type="number" min="1" /></div>
</template>

<style scoped>
input[type='number']:focus-visible,
input[type='number']:focus-within,
input[type='number']:focus {
    min-width: v-bind(minWidth);
}
</style>
