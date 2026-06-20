<script lang="ts" setup>
import { OriCard } from '@oriui/vue'
import { useCanvasToolsStore } from '@modules/canvas'
import { computed } from 'vue'

const toolsStore = useCanvasToolsStore()

const lineWeight = computed({
    get() {
        return toolsStore.tool?.lineWeight ?? 1
    },
    set(value) {
        toolsStore.setLineWeight(value)
    }
})
</script>

<template>
    <div v-if="false" class="draw-settings__size-preview">
        <div></div>
    </div>
    <OriCard v-if="false" class="draw-settings__size-modal">
        <div class="draw-settings__item">
            <label for="lineWeight">Line weight</label>
            <input v-model.lazy.trim="lineWeight" min="1" name="lineWeight" type="number" />
        </div>
    </OriCard>
</template>

<style></style>
