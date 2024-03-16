<script lang="ts" setup>
import { useCanvasStore, useToolsStore } from '@shared/stores'
import { ToolbarIcon } from '@shared/ui'

const canvasStore = useCanvasStore()
const toolsStore = useToolsStore()

const toolsNamesBySVGPathValues: Record<string, string> = {
    Eraser: 'M16.24,3.56L21.19,8.5C21.97,9.29 21.97,10.55 21.19,11.34L12,20.53C10.44,22.09 7.91,22.09 6.34,20.53L2.81,17C2.03,16.21 2.03,14.95 2.81,14.16L13.41,3.56C14.2,2.78 15.46,2.78 16.24,3.56M4.22,15.58L7.76,19.11C8.54,19.9 9.8,19.9 10.59,19.11L14.12,15.58L9.17,10.63L4.22,15.58Z',
    Pen: 'M20.71,7.04C20.37,7.38 20.04,7.71 20.03,8.04C20,8.36 20.34,8.69 20.66,9C21.14,9.5 21.61,9.95 21.59,10.44C21.57,10.93 21.06,11.44 20.55,11.94L16.42,16.08L15,14.66L19.25,10.42L18.29,9.46L16.87,10.87L13.12,7.12L16.96,3.29C17.35,2.9 18,2.9 18.37,3.29L20.71,5.63C21.1,6 21.1,6.65 20.71,7.04M3,17.25L12.56,7.68L16.31,11.43L6.75,21H3V17.25Z',
    Line: 'M15,3V7.59L7.59,15H3V21H9V16.42L16.42,9H21V3M17,5H19V7H17M5,17H7V19H5',
    Square: 'M3,3H21V21H3V3M5,5V19H19V5H5Z',
    Circle: 'M12,20A8,8 0 0,1 4,12A8,8 0 0,1 12,4A8,8 0 0,1 20,12A8,8 0 0,1 12,20M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2Z',
    Triangle: 'M12,2L1,21H23M12,6L19.53,19H4.47'
}

function getIsActive(ToolClass: (typeof toolsStore.toolsClasses)[number]) {
    return ToolClass.name === toolsStore.tool?.constructor.name
}

function handleClick(ToolClass: (typeof toolsStore.toolsClasses)[number]) {
    if (!getIsActive(ToolClass) && canvasStore.canvas) {
        toolsStore.setTool(new ToolClass(canvasStore.canvas))
    }
}
</script>

<template>
    <template v-if="canvasStore.canvas">
        <ToolbarIcon
            v-for="ToolClass in toolsStore.toolsClasses"
            :key="ToolClass.name"
            :icon-path-value="toolsNamesBySVGPathValues[ToolClass.name]"
            :is-active="getIsActive(ToolClass)"
            :title="ToolClass.name"
            class="toolbar__paint-tool"
            @click="handleClick(ToolClass)"
        />
    </template>
</template>
