import { CanvasHistory } from '@shared/lib'
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export const useCanvasHistoryStore = defineStore('history', () => {
    const historyHandler = ref<CanvasHistory | null>(null)

    function setHistoryHandler(value: CanvasHistory) {
        historyHandler.value = value
    }

    const paintHistoryList = ref([])

    function makeHistoryStep() {
        paintHistoryList.value.push()
    }

    return { historyHandler, setHistoryHandler }
})
