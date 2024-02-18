<script setup lang="ts">
import { useToolsStore } from '@shared/stores';
import { computed } from 'vue';

const toolsStore = useToolsStore();

const color = computed({
    get() {
        return toolsStore.color;
    },
    set(value) {
        toolsStore.setColor(value);
    }
});

const lineWeight = computed({
    get() {
        return toolsStore.lineWeight;
    },
    set(value) {
        toolsStore.setLineWeight(value);
    }
});

const minWidth = computed(() => `${lineWeight.value}`.length + 4 + 'ch');
</script>

<template>
    <input v-model="color" title="Color picker" type="color" />
    <div title="Line weight"><input v-model.trim="lineWeight" type="number" min="1" /></div>
</template>

<style scoped>
input[type='number']:focus-visible,
input[type='number']:focus-within,
input[type='number']:focus {
    min-width: v-bind(minWidth);
}
</style>
