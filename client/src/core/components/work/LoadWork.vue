<script setup lang="ts">
import { VButton } from 'vueinjar'
import { icons, useWorksStore } from '@core'

const worksStore = useWorksStore()

function handleLoadWorks() {
    worksStore.fetchWorks()
}
</script>

<template>
    <v-button
        :loading="worksStore.isSaving"
        :icon="icons.work.load"
        text="Load"
        radius="md"
        variant="tonal"
        fluid
        @click="handleLoadWorks"
    />
</template>
