<script setup lang="ts">
import { VButton } from 'vueinjar'
import { icons, useWorksStore } from '@core'

const worksStore = useWorksStore()
</script>

<template>
    <v-button
        :loading="worksStore.isGetting"
        :icon="icons.work.load"
        text="Load"
        radius="md"
        variant="tonal"
        fluid
        @click="worksStore.fetchWorks"
    />
</template>
