<script setup lang="ts">
import { OriButton } from '@oriui/vue'
import { icons } from '@core'
import { useCanvasHistoryStore, useCanvasStore } from '../stores'

const canvasStore = useCanvasStore()
const historyStore = useCanvasHistoryStore()
</script>

<template>
    <template v-if="canvasStore.canvas">
        <OriButton
            :icon="icons.art.undo"
            :disabled="!historyStore.historyHandler?.hasPrevStep"
            :active="historyStore.historyHandler?.hasPrevStep"
            class="history-handler"
            variant="plain"
            size="lg"
            @click="historyStore.undo"
        />
        <OriButton
            :icon="icons.art.redo"
            :disabled="!historyStore.historyHandler?.hasNextStep"
            :active="historyStore.historyHandler?.hasNextStep"
            class="history-handler"
            variant="plain"
            size="lg"
            @click="historyStore.redo"
        />
    </template>
</template>
