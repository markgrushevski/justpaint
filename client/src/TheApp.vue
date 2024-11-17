<script lang="ts" setup>
import { CopyArt, LayersHandler, LoadArt, MenuToggler, RenameArt, SaveArt, ThemeToggler, UserProfile } from '@core'
import { DrawHistoryHandlers, DrawSettings, DrawTools, TheCanvas, useCanvasStore } from '@modules/canvas'

const canvasStore = useCanvasStore()
</script>

<template>
    <LayersHandler v-if="false" />

    <MenuToggler>
        <template #title>
            <RenameArt />
        </template>
        <template #main>
            <CopyArt :canvas="canvasStore.canvas" />
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
        <DrawSettings />
        <DrawTools />
    </div>
</template>

<style>
@import 'reset.css';
@import 'main.css';

.app__header {
    position: relative;

    height: var(--v-size-action_lg);

    display: flex;
    align-items: center;
    justify-content: center;

    background-color: var(--v-color_surface);
}

.app__main {
    height: calc(100dvh - var(--v-size-action_lg) - var(--v-size-action_lg));

    display: flex;
    align-items: center;
    justify-content: center;
}

.app__footer {
    height: var(--v-size-action_lg);

    display: flex;
    align-items: center;
    justify-content: center;

    background-color: var(--v-color_surface);
}

@media (width >= 600px) {
    .app__main {
        height: calc(100dvh - var(--v-size-action_lg) - var(--v-size-action_xl));
    }

    .app__footer {
        height: var(--v-size-action_xl);
    }

    .app__footer .v-button {
        --v-size-action: var(--v-size-action_xl);
    }
}
</style>
