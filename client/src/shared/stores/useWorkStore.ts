import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useWorkStore = defineStore('work', () => {
    const workHandlers = ref(['Undo', 'Redo', 'Save']);
    return { workHandlers };
});
