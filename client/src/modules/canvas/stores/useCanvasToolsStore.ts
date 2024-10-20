import { defineStore } from 'pinia'
import { ref } from 'vue'
import { type CanvasHistory, Circle, Eraser, Line, Pen, Square, type ToolClass } from '../models'

export const useCanvasToolsStore = defineStore('tools', () => {
    const toolClasses = ref<ToolClass[]>([Eraser, Pen, Line, Circle, Square])
    const tool = ref<InstanceType<ToolClass>>()

    function setTool(
        toolClass: (typeof toolClasses.value)[number],
        canvas: HTMLCanvasElement,
        canvasHistory: CanvasHistory
    ) {
        tool.value = new toolClass(canvas, canvasHistory)
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
