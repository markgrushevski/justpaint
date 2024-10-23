<script setup lang="ts">
import { VButton } from 'vueinjar'
import { icons, useWorksStore } from '@core'

const worksStore = useWorksStore()
</script>

<template>
    <v-button
        :loading="worksStore.isSaving"
        :icon="icons.work.save"
        text="Save"
        radius="md"
        variant="tonal"
        fluid
        @click="worksStore.saveWork"
    />
</template>
