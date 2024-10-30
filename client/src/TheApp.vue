<script setup lang="ts">
import { MenuToggler, ThemeToggler, SaveArt, LoadArt, CopyArt, LayersHandler, UserProfile, RenameArt } from '@core'
import { TheCanvas, DrawTools, DrawHistoryHandlers, useCanvasStore } from '@modules/canvas'
</script>

<template>
    <LayersHandler />

    <MenuToggler>
        <template #title>
            <RenameArt />
        </template>
        <template #main>
            <CopyArt :canvas="useCanvasStore().canvas" />
            <SaveArt />
            <LoadArt />
        </template>
        <template #footer>
            <UserProfile />
            <ThemeToggler />
        </template>
    </MenuToggler>

    <div class="app__header">
        <DrawHistoryHandlers />
    </div>
    <div class="app__main">
        <TheCanvas />
    </div>
    <div class="app__footer">
        <!--<DrawSettings />-->
        <DrawTools />
    </div>
</template>

<style>
@import 'reset.css';
@import 'main.css';

.app__header {
    position: relative;

    height: 48px;

    display: flex;
    align-items: center;
    justify-content: center;

    background-color: var(--v-color_surface);
}

.app__main {
    height: calc(100dvh - 48px - 60px);

    display: flex;
    align-items: center;
    justify-content: center;
}

.app__footer {
    height: 60px;

    display: flex;
    align-items: center;
    justify-content: center;

    background-color: var(--v-color_surface);
}
</style>
