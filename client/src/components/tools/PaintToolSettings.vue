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
</script>

<template>
    <input v-model="color" title="Color picker" type="color" />
    <div><span>Line weight:&nbsp;</span><input v-model.trim="lineWeight" type="number" min="1" /></div>
</template>
