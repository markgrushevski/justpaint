<script setup lang="ts">
import { useToolsStore } from '@shared/stores'
import { computed } from 'vue'

const toolsStore = useToolsStore()

const strokeColor = computed({
    get() {
        return toolsStore.tool?.strokeColor ?? '#000000'
    },
    set(value) {
        toolsStore.setStrokeColor(value)
    }
})

const fillColor = computed({
    get() {
        return toolsStore.tool?.fillColor ?? '#000000'
    },
    set(value) {
        toolsStore.setFillColor(value)
    }
})

const lineWeight = computed({
    get() {
        return toolsStore.tool?.lineWeight ?? 1
    },
    set(value) {
        toolsStore.setLineWeight(value)
    }
})

// const minWidth = computed(() => `${lineWeight.value}`.length + 4 + 'ch')
</script>

<template>
    <div class="draw-settings-bar__item">
        <label for="fillColor">Fill color</label>
        <input v-model.lazy="fillColor" type="color" name="fillColor" />
    </div>
    <div class="draw-settings-bar__item">
        <label for="strokeColor">Stroke color</label>
        <input v-model.lazy="strokeColor" type="color" name="strokeColor" />
    </div>
    <div class="draw-settings-bar__item">
        <label for="lineWeight">Line weight</label>
        <input v-model.lazy.trim="lineWeight" type="number" min="1" name="lineWeight" />
    </div>
</template>

<style>
.draw-settings-bar__item {
    width: 100%;
}

.draw-settings-bar__item label {
    display: block;
    margin-bottom: 2px;
}

.draw-settings-bar__item input {
    border: 0;

    border-radius: var(--border-radius-controls);

    color: var(--color-text);

    outline-width: 2px;
    outline-color: var(--color-accent);

    background-color: var(--color-background);

    cursor: pointer;
}

.draw-settings-bar__item input[type='color'] {
    width: 4em;
    height: 3em;
}

.draw-settings-bar__item input[type='number'] {
    padding: 3px 7px;
    width: 4em;
}

.draw-settings-bar__item input:focus-visible,
.draw-settings-bar__item input:focus-within,
.draw-settings-bar__item input:focus {
    outline-style: solid;
}

/* .draw-settings-bar__item input[type='number']:focus-visible,
.draw-settings-bar__item input[type='number']:focus-within,
.draw-settings-bar__item input[type='number']:focus {
    min-width: v-bind(minWidth);
} */
</style>
