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
    <div class="toolbar__paint-tool-settings">
        <input v-model="color" title="Color picker" type="color" />
        <h3>Line weight in pixels:&nbsp;<input v-model.trim="lineWeight" type="number" min="1" /></h3>
    </div>
</template>

<style>
.toolbar__paint-tool-settings {
    display: flex;
    align-items: center;
    gap: 12px;
}

.toolbar__paint-tool-settings input {
    font-size: 16px;

    border: 0;

    border-radius: 1px;

    color: var(--color-text);
    outline-color: var(--color-text);
    background-color: var(--color-tool);
}

.toolbar__paint-tool-settings input[type='color'] {
    width: 3rem;
    height: 3rem;
}

.toolbar__paint-tool-settings input[type='number'] {
    padding: 3px 7px;
    width: 60px;
}

.toolbar__paint-tool-settings input:focus-visible,
.toolbar__paint-tool-settings input:focus-within,
.toolbar__paint-tool-settings input:focus {
    outline-style: solid;
}
</style>
