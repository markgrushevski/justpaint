<script setup lang="ts">
import { ref } from 'vue'
import { VButton } from 'vueinjar'
import { icons } from '@core'

function handleSaveWork() {}
</script>

<template>
    <v-button :icon="icons.work.save" text="Save" radius="md" variant="tonal" fluid @click="handleSaveWork" />
</template>
