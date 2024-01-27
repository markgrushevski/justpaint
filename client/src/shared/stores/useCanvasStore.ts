import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useCanvasStore = defineStore('canvas', () => {
    const canvas = ref<HTMLCanvasElement | null>(null);

    function setCanvas(element: HTMLCanvasElement | undefined) {
        if (element) canvas.value = element;
        else console.error("canvas value hasn't set");
    }
    return { canvas, setCanvas };
});
