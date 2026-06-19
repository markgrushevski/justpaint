<script setup lang="ts">
import { OriButton, OriCard, OriIcon } from '@oriui/ui'
import { icons } from '../../constants'
import { ref } from 'vue'
import { useArtsStore } from '../../stores'

const artsStore = useArtsStore()

function updateArtName(ev: Event) {
    const el = ev.target as HTMLElement
    artsStore.art.name = el.innerText
}
</script>

<template>
    <span style="min-width: 3ch" contenteditable @input="updateArtName">{{ artsStore.art.name }}</span>
    <OriIcon :icon="icons.art.rename" inline />
</template>

<style></style>
