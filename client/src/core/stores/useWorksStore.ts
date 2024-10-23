import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { API, type Work } from '@core'

export const useWorksStore = defineStore('works', () => {
    const works = ref<Work[]>([])

    const isGetting = ref(false)
    const isSaving = ref(false)

    const isLoading = computed(() => isGetting.value || isSaving.value)

    async function fetchWorks() {
        if (isLoading.value) return

        isGetting.value = true
        await API.works.getWorks()
        isGetting.value = false
    }

    async function saveWork() {
        if (isLoading.value) return

        isSaving.value = true
        await API.works.saveWork({} as any)
        isSaving.value = false
    }

    return { works, isGetting, isSaving, isLoading, fetchWorks, saveWork }
})
