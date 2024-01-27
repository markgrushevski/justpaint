import type { Tool } from '@shared/types';
import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useToolsStore = defineStore('tools', () => {
    const currentTool = ref<Tool>('pen');

    const lineWeight = ref(1);

    const color = ref('#000000');

    function setTool(value: Tool) {
        currentTool.value = value;
    }

    function setLineWeight(value: number) {
        if (value > 0) lineWeight.value = value;
    }

    function setColor(value: string) {
        if (value?.match(/#[\da-f]{6}/i)) color.value = value;
    }

    return { currentTool, lineWeight, color, setTool, setLineWeight, setColor };
});
