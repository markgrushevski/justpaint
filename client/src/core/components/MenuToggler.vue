<script setup lang="ts">
import { ref } from 'vue'
import { mdiMenu, mdiMenuOpen } from '@mdi/js'
import { VButton, VCard } from 'vueinjar'

const isOpen = ref(false)
</script>

<template>
    <v-button
        :icon="isOpen ? mdiMenuOpen : mdiMenu"
        class="main-menu-toggler"
        size="lg"
        variant="plain"
        @click="isOpen = !isOpen"
    />
    <v-card :class="{ 'main-menu_open v-shadow': isOpen }" class="main-menu" color="background" radius="zero">
        <template #title><slot name="title"></slot></template>
        <template #body><slot name="main"></slot></template>
        <template #actions-append><slot name="footer"></slot></template>
    </v-card>
</template>

<style>
.main-menu-toggler {
    position: absolute;
    top: 0;
    right: 0;
    z-index: 12;
}

.main-menu {
    overflow-y: auto;

    position: fixed;
    top: 0;
    right: 0;
    z-index: 10;

    padding: 12px;

    max-width: 100dvw;
    width: 400px;
    height: 100dvh;

    display: flex;
    flex-direction: column;
    gap: var(--v-size-gap_xl);

    border-left: 1px solid var(--v-color_primary);

    background-color: var(--v-color_surface);

    transform: translateX(101%);

    transition: transform ease-out 0.25s;
}

.main-menu.main-menu_open {
    transform: translateX(0);
}

.main-menu.v-card .v-card__header {
    height: var(--v-size-action_sm);
}

.main-menu.v-card .v-card__header * {
    max-height: var(--v-size-action_sm);
}

.main-menu.v-card .v-card__title {
    display: inline-flex;
    align-items: center;

    line-height: 1;

    cursor: pointer;
}

.main-menu.v-card .v-card__body {
    overflow-y: auto;
    overflow-x: hidden;
    display: flex;
    flex-grow: 1;
    flex-direction: column;
    gap: var(--v-size-gap);
}

.main-menu.v-card .v-card__actions {
    align-items: flex-end;
    justify-content: space-between;
}
</style>
