<script setup lang="ts">
import { OriButton } from '@oriui/ui'
import { mdiLayers } from '@mdi/js'
</script>

<template>
    <OriButton :icon="mdiLayers" class="layers-handler" size="lg" variant="plain" />
</template>

<style>
.layers-handler {
    position: absolute;
    top: 0;
    left: 0;
    z-index: 8;
}
</style>
