<script lang="ts" setup>
import { useCanvasToolsStore } from '../../stores'
import ColorSettings from './ColorSettings.vue'
import SizeSettings from './SizeSettings.vue'

const toolsStore = useCanvasToolsStore()

// const minWidth = computed(() => `${lineWeight.value}`.length + 4 + 'ch')
</script>

<template>
    <div v-if="toolsStore.tool" class="draw-settings">
        <SizeSettings />
        <ColorSettings />
    </div>
</template>

<style>
.draw-settings {
    position: relative;
    height: 100%;
}

.draw-settings__item {
    /*width: 100%;*/
}

.draw-settings__item label {
    display: block;
    margin-bottom: 2px;
    text-align: center;
}

.draw-settings__item input {
    background-color: transparent;
}

.draw-settings__item input[type='color'] {
    height: 3em;
}

.draw-settings__item input[type='number'] {
    padding: 3px 7px;
    max-width: 100%;
}

/* .draw-settings__item input:focus-visible,
.draw-settings__item input:focus-within,
.draw-settings__item input:focus {
    outline-style: solid;
} */

/* .draw-settings__item input[type='number']:focus-visible,
.draw-settings__item input[type='number']:focus-within,
.draw-settings__item input[type='number']:focus {
    min-width: v-bind(minWidth);
} */
</style>
