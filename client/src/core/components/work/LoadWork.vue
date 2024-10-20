<script setup lang="ts">
import { ref } from 'vue'
import { VButton } from 'vueinjar'
import { icons } from '@core'

function handleLoadWork() {}
</script>

<template>
    <v-button :icon="icons.work.load" text="Load" radius="md" variant="tonal" fluid @click="handleLoadWork" />
</template>
