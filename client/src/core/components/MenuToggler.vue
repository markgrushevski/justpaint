<script setup lang="ts">
import { mdiMenu, mdiMenuOpen } from '@mdi/js'
import { VButton, VCard } from 'vueinjar'

const isOpen = defineModel<boolean>()
</script>

<template>
    <v-button
        :icon="isOpen ? mdiMenuOpen : mdiMenu"
        class="menu-toggler"
        size="lg"
        color="surface"
        fluid
        @click="isOpen = !isOpen"
    />
    <v-card
        :class="{ menu_open: isOpen }"
        class="menu v-shadow"
        color="background"
        radius="zero"
        title="New work"
        reverse-appended-actions
    >
        <template #body><slot name="main"></slot></template>
        <template #actions-append><slot name="footer"></slot></template>
    </v-card>
</template>

<style>
.menu-toggler {
    position: fixed;
    top: 0;
    right: 0;
    z-index: 12;

    margin: var(--v-size-gap);
}

.menu {
    position: fixed;
    top: 0;
    right: 0;
    z-index: 10;

    max-width: 100vw;
    width: 400px;
    height: 100vh;

    display: flex;
    flex-direction: column;
    gap: var(--v-size-gap_xl);

    border-left: 1px solid var(--v-color_primary);

    background-color: var(--v-color_surface);

    transform: translateX(100%);

    transition: transform ease-out 0.25s;
}

.menu.menu_open {
    transform: translateX(0);
}

.v-card__body {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    gap: var(--v-size-gap);
}
</style>
