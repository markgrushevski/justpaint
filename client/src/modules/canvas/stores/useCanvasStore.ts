import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useCanvasStore = defineStore('canvas', () => {
    const canvas = ref<HTMLCanvasElement | null>(null)

    return { canvas }
})
