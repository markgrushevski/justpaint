import { defineStore } from 'pinia'
import { type Ref, ref } from 'vue'
import type { CanvasHistory } from '../models'
import { useCanvasToolsStore } from '../stores'

export const useCanvasHistoryStore = defineStore('history', () => {
    const historyHandler: Ref<CanvasHistory | null> = ref(null)

    function setHistoryHandler(value: CanvasHistory) {
        historyHandler.value = value
    }

    async function undo() {
        const tool = useCanvasToolsStore().tool
        const step = historyHandler.value?.stepBack()
        if (tool && step) {
            await tool.loadStateToCanvas(step.canvasDataURL)
        }
    }

    async function redo() {
        const tool = useCanvasToolsStore().tool
        const step = historyHandler.value?.stepForward()
        if (tool && step) {
            await tool.loadStateToCanvas(step.canvasDataURL)
        }
    }

    return { historyHandler, setHistoryHandler, undo, redo }
})
