<script setup lang="ts">
import { onMounted } from 'vue'
import { MenuToggler, ThemeToggler, SaveWork, LoadWork, CopyWork, LayersHandler } from '@core'
import { TheCanvas, DrawTools, DrawHistoryHandlers } from '@modules/canvas'

onMounted(() => {
    /*API.getWorks().then((res) => {
        console.group('getWorks')
        console.log(res)
        console.groupEnd()
    }) 
     API.saveWork().then((res) => {
        console.group('saveWork')
        console.log(res)
        console.groupEnd()
    }) */
})
</script>

<template>
    <LayersHandler />

    <MenuToggler>
        <template #main>
            <SaveWork />
            <LoadWork />
            <CopyWork />
        </template>
        <template #footer>
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
    height: calc(100vh - 48px - 60px);

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
