import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { mainAPI, type Work } from '@core'

export const useWorksStore = defineStore('works', () => {
    const currentWork = ref<Work>({ name: 'new art', createdAt: '' })
    const works = ref<Work[]>([])

    const isGetting = ref(false)
    const isSaving = ref(false)

    const isLoading = computed(() => isGetting.value || isSaving.value)

    async function fetchWorks() {
        if (isLoading.value) return

        isGetting.value = true
        const result = await mainAPI.works.getWorks()
        isGetting.value = false

        console.log({ result })
    }

    async function saveWork() {
        if (isLoading.value) return

        isSaving.value = true
        await mainAPI.works.saveWork({} as any)
        isSaving.value = false
    }

    return { works, isGetting, isSaving, isLoading, fetchWorks, saveWork }
})
