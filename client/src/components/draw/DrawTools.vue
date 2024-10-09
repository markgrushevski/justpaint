<script lang="ts" setup>
import { useCanvasStore, useToolsStore } from '@shared/stores'
import { WorkIcon } from '@shared/ui'

const canvasStore = useCanvasStore()
const toolsStore = useToolsStore()

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
        <WorkIcon
            v-for="ToolClass in toolsStore.toolClasses"
            :key="ToolClass.name"
            :is-active="getIsActive(ToolClass)"
            :icon-name="ToolClass.name"
            :title="ToolClass.name"
            class="draw-tools-bar__item"
            @click="handleClick(ToolClass)"
        />
    </template>
</template>
