<script lang="ts" setup>
import { OriButton, OriCard } from '@oriui/vue'
import { useCanvasStore, useCanvasToolsStore } from '@modules/canvas'
import { computed, ref, watch } from 'vue'
import { onClickOutside } from '@vueuse/core'

const toolsStore = useCanvasToolsStore()
const canvasStore = useCanvasStore()

const colorModal = ref(null)
const colorModalToggler = ref(null)
const isOpen = ref(false)

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

onClickOutside(
    colorModal,
    () => {
        isOpen.value = false
    },
    { ignore: [colorModalToggler] }
)

watch(isOpen, () => {
    if (isOpen.value) canvasStore.blockCanvas()
    else canvasStore.unblockCanvas()
})
</script>

<template>
    <OriButton
        ref="colorModalToggler"
        class="draw-settings__color-preview"
        radius="zero"
        size="lg"
        title="Color"
        variant="text"
        @click="isOpen = !isOpen"
    >
        <div><div :style="`background-color: ${strokeColor};`"></div></div>
        <div><div :style="`background-color: ${fillColor};`"></div></div>
    </OriButton>
    <OriCard v-show="isOpen" ref="colorModal" class="draw-settings__color-modal" radius="zero">
        <div class="draw-settings__item">
            <label for="strokeColor">Stroke</label>
            <input v-model.lazy="strokeColor" name="strokeColor" type="color" />
        </div>
        <div class="draw-settings__item">
            <label for="fillColor">Fill</label>
            <input v-model.lazy="fillColor" name="fillColor" type="color" />
        </div>
    </OriCard>
</template>

<style>
button.ori-button.draw-settings__color-preview {
    width: var(--ori-size-action);
    height: 100%;

    display: flex;
    align-items: center;
    justify-content: center;
    flex-direction: column;
    gap: 0;

    cursor: pointer;
    border-radius: 50%;
    background-color: transparent;
}

:root.dark button.ori-button.draw-settings__color-preview {
    outline-color: white;
}

button.ori-button.draw-settings__color-preview > div {
    position: relative;
    width: 50%;
    height: 25%;
    overflow: hidden;
}

button.ori-button.draw-settings__color-preview > div > div {
    position: absolute;
    width: 100%;
    height: 200%;
    border-radius: 50%;
}

button.ori-button.draw-settings__color-preview > div:first-of-type > div {
    top: 0;
}

button.ori-button.draw-settings__color-preview > div:last-of-type > div {
    bottom: 0;
}

.draw-settings__color-modal {
    position: absolute;
    bottom: 100%;

    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_xl);

    box-shadow: var(--ori-shadow-lg);
}
</style>
