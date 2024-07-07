import { Eraser, Pen, Line, Circle, Square, type Tool } from '@shared/lib'
import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useToolsStore = defineStore('tools', () => {
    const toolsClasses = ref([Eraser, Pen, Line, Circle, Square])
    const tool = ref<Tool>()

    function setTool(value: Tool) {
        tool.value = value
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

    return { toolsClasses, tool, setTool, setLineWeight, setStrokeColor, setFillColor }
})
