import { CanvasHistory } from '@shared/lib'
import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useCanvasHistoryStore = defineStore('history', () => {
    const historyHandler = ref<CanvasHistory | null>(null)

    const paintHistoryList = ref([])

    function setHistoryHandler(value: CanvasHistory) {
        historyHandler.value = value
    }

    function makeHistoryStep() {
        paintHistoryList.value.push()
    }

    return { historyHandler, setHistoryHandler }
})
