<script lang="ts" setup>
import { VButton } from 'vueinjar'
import { icons } from '@core'
import { useCanvasStore, useCanvasToolsStore } from '../stores'

const canvasStore = useCanvasStore()
const toolsStore = useCanvasToolsStore()

function getIsActive(ToolClass: (typeof toolsStore.toolClasses)[number]) {
    return ToolClass.name === toolsStore.tool?.constructor.name
}

function handleClick(ToolClass: (typeof toolsStore.toolClasses)[number]) {
    if (!getIsActive(ToolClass) && canvasStore.canvas) {
        toolsStore.setTool(ToolClass, canvasStore.canvas)
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
            color="surface"
            fluid
            class="draw-tools-bar__item"
            @click="handleClick(ToolClass)"
        />
    </template>
</template>
