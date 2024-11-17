import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useCanvasStore = defineStore('canvas', () => {
    const canvas = ref<HTMLCanvasElement | null>(null)

    function blockCanvas() {
        if (canvas.value) {
            canvas.value.classList.add('blocked')
        }
    }

    function unblockCanvas() {
        if (canvas.value) {
            canvas.value.classList.remove('blocked')
        }
    }

    return { canvas, blockCanvas, unblockCanvas }
})
