<script setup lang="ts">
import { VButton } from 'vueinjar'
import { icons } from '@core'
import { useCanvasHistoryStore, useCanvasStore } from '../stores'

const canvasStore = useCanvasStore()
const historyStore = useCanvasHistoryStore()
</script>

<template>
    <template v-if="canvasStore.canvas">
        <v-button
            :icon="icons.art.undo"
            :disabled="!historyStore.historyHandler?.hasPrevStep"
            :active="historyStore.historyHandler?.hasPrevStep"
            class="history-handler"
            variant="plain"
            size="lg"
            @click="historyStore.undo"
        />
        <v-button
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
