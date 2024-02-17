import { CanvasTool, Eraser, Pen, Square } from '@shared/lib';
import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useToolsStore = defineStore('tools', () => {
    const toolsClasses = ref([Eraser, Pen, Square]);
    const tool = ref<CanvasTool | null>(null);

    const lineWeight = ref(1);

    const color = ref('#000000');

    function setTool(value: CanvasTool) {
        tool.value = value;
    }

    function setLineWeight(value: number) {
        if (value > 0) lineWeight.value = value;
    }

    function setColor(value: string) {
        if (value.match(/#[\da-f]{6}/i)) color.value = value;
    }

    return { toolsClasses, tool, lineWeight, color, setTool, setLineWeight, setColor };
});
