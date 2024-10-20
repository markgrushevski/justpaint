<script setup lang="ts">
import { VButton } from 'vueinjar'
import { mdiLayers } from '@mdi/js'
</script>

<template>
    <v-button :icon="mdiLayers" class="layers-handler" size="lg" variant="plain" />
</template>

<style>
.layers-handler {
    position: absolute;
    top: 0;
    left: 0;
    z-index: 8;
}
</style>
