import { injectionKeys } from '@shared/constants'
import { Eraser, Pen, Line, Circle, Square, type ToolClass } from '@shared/lib'
import { defineStore } from 'pinia'
import { inject, ref } from 'vue'

export const useToolsStore = defineStore('tools', () => {
    const toolClasses = ref<ToolClass[]>([Eraser, Pen, Line, Circle, Square])
    const tool = ref<InstanceType<ToolClass>>()

    function setTool(toolClass: (typeof toolClasses.value)[number], canvas: HTMLCanvasElement) {
        tool.value = new toolClass(canvas)
    }

    function setLineWeight(value: number) {
        if (tool.value && value > 0) {
            tool.value.lineWeight = value
        }
    }

    function setStrokeColor(value: string) {
        if (tool.value && value.match(/#[\da-f]{6}/i)) {
            tool.value.strokeColor = value
        }
    }

    function setFillColor(value: string) {
        if (tool.value && value.match(/#[\da-f]{6}/i)) {
            tool.value.fillColor = value
        }
    }

    return { toolClasses, tool, setTool, setLineWeight, setStrokeColor, setFillColor }
})
