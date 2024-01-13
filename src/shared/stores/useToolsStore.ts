import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useToolsStore = defineStore('tools', () => {
    const currentTool = ref(null);

    return { currentTool };
});
