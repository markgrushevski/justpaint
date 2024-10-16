<script setup lang="ts">
// import { API } from '@shared/lib'
import { TheCanvas, DrawTools, DrawSettings } from '@modules/canvas'
import { MenuToggler, ThemeToggler, SaveWork, LoadWork, CopyWork } from '@core'

/*onMounted(() => {
     API.getWorks().then((res) => {
        console.group('getWorks')
        console.log(res)
        console.groupEnd()
    }) 
     API.saveWork().then((res) => {
        console.group('saveWork')
        console.log(res)
        console.groupEnd()
    }) 
})*/
</script>

<template>
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
    <DrawTools />
    <DrawSettings />
    <TheCanvas />
</template>

<style>
*,
*::before,
*::after {
    box-sizing: border-box;
}

* {
    margin: 0;
    padding: 0;
    border: 0;
    font-family: 'Nunito', sans-serif;
    font-optical-sizing: auto;
    font-weight: 500;
    font-style: normal;
}

html,
body,
#app {
    font-size: 16px;
}

body {
    margin: 0;

    width: 100vw;
    height: 100vh;
}

input {
    font-size: 1rem;
}

:root {
    --v-color_primary-light: hsl(20, 100%, 50%);
    --v-color_primary-dark: hsl(20, 100%, 60%);
}

#app {
    overflow: hidden;

    position: relative;

    max-width: 100vw;
    max-height: 100vh;
    width: 100%;
    height: 100%;

    color: var(--v-color_primary);
    background-color: var(--v-color_background);
}

#app > nav {
    position: absolute;
    top: 0;
    right: 0;

    display: flex;
    align-items: center;
    flex-direction: column;
    gap: var(--v-size-gap_lg);
}
</style>
