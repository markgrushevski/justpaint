import { CanvasTool, Eraser, Pen, Line, Circle, Triangle, Square } from '@shared/lib';
import { defineStore } from 'pinia';
import { computed, ref } from 'vue';

export const useToolsStore = defineStore('tools', () => {
    const toolsClasses = ref([Eraser, Pen, Line, Circle, Triangle, Square]);
    const tool = ref<CanvasTool | null>(null);

    function setTool(value: CanvasTool) {
        tool.value = value;
    }

    function setLineWeight(value: number) {
        if (tool.value && value > 0) {
            tool.value.lineWeight = value;
        }
    }

    function setColor(value: string) {
        if (tool.value && value.match(/#[\da-f]{6}/i)) {
            tool.value.strokeColor = value;
            tool.value.fillColor = value;
        }
    }

    return { toolsClasses, tool, setTool, setLineWeight, setColor };
});
