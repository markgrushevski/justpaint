import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

export const useWorkStore = defineStore('work', () => {
    const workHandlers = ref(['Undo', 'Redo', 'Save', 'Load'])

    const paintHistoryList = ref([])

    function makeHistoryStep() {
        paintHistoryList.value.push()
    }

    return { workHandlers }
})
