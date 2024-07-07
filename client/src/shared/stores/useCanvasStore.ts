import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useCanvasStore = defineStore('canvas', () => {
    const canvas = ref<HTMLCanvasElement | null>(null)

    const canvasWidth = ref(200)
    const canvasHeight = ref(200)

    function setCanvas(element: HTMLCanvasElement) {
        canvas.value = element
    }

    return { canvas, canvasWidth, canvasHeight, setCanvas }
})
