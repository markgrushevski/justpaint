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
        variant="plain"
        @click="isOpen = !isOpen"
    />
    <v-card
        :class="{ 'menu_open v-shadow': isOpen }"
        class="main-menu"
        color="background"
        radius="zero"
        title="New work"
    >
        <template #body><slot name="main"></slot></template>
        <template #actions-append><slot name="footer"></slot></template>
    </v-card>
</template>

<style>
.menu-toggler {
    position: absolute;
    top: 0;
    right: 0;
    z-index: 12;
}

.main-menu {
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

    transform: translateX(101%);

    transition: transform ease-out 0.25s;
}

.main-menu.menu_open {
    transform: translateX(0);
}

.v-card__body {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    gap: var(--v-size-gap);
}

.main-menu.v-card .v-card__actions {
    align-items: flex-end;
    justify-content: space-between;
}
</style>
