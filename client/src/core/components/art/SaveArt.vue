<script setup lang="ts">
import { ref } from 'vue'
import { VButton } from 'vueinjar'
import { icons, mainAPI, useArtsStore, useUserStore } from '@core'
import { getCanvasDataURL, useCanvasStore } from '@modules/canvas'

const usersStore = useUserStore()
const artsStore = useArtsStore()
const canvasStore = useCanvasStore()

const isSaving = ref(false)

async function handleSaveArt() {
    try {
        isSaving.value = true

        artsStore.art.layers[0].dataURL = getCanvasDataURL(useCanvasStore().canvas!)
        console.log('save', artsStore.art)
        const success = await mainAPI.arts.saveArt(artsStore.art)
        console.log('saved', success)
    } catch (e) {
        console.error(e)
    } finally {
        isSaving.value = false
    }
}
</script>

<template>
    <v-button
        :loading="isSaving"
        :icon="icons.art.save"
        :disabled="!usersStore.isLoggedIn || isSaving"
        text="Save"
        radius="md"
        variant="tonal"
        fluid
        @click="handleSaveArt"
    />
</template>
