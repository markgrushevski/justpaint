<script lang="ts" setup>
import { VButton } from 'vueinjar'
import { icons } from '@core'
import { useCanvasHistoryStore, useCanvasStore, useCanvasToolsStore } from '../stores'

const canvasStore = useCanvasStore()
const historyStore = useCanvasHistoryStore()
const toolsStore = useCanvasToolsStore()

function getIsActive(ToolClass: (typeof toolsStore.toolClasses)[number]) {
    return ToolClass.name === toolsStore.tool?.constructor.name
}

function handleClick(ToolClass: (typeof toolsStore.toolClasses)[number]) {
    if (!getIsActive(ToolClass) && canvasStore.canvas && historyStore.historyHandler) {
        toolsStore.setTool(ToolClass, canvasStore.canvas, historyStore.historyHandler)
    }
}
</script>

<template>
    <template v-if="canvasStore.canvas">
        <v-button
            v-for="ToolClass in toolsStore.toolClasses"
            :key="ToolClass.name"
            :icon="icons.draw[ToolClass.name]"
            :title="ToolClass.name"
            :active="getIsActive(ToolClass)"
            size="xl"
            radius="zero"
            variant="plain"
            @click="handleClick(ToolClass)"
        />
    </template>
</template>
