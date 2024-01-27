<script setup lang="ts">
import { PaintTools, TheCanvas, PaintToolSettings, WorkHandlers } from '@components';
</script>

<template>
    <header class="toolbar">
        <div class="toolbar__line-1">
            <PaintTools />
            <WorkHandlers />
        </div>

        <div class="toolbar__line-2">
            <PaintToolSettings />
        </div>
    </header>
    <main class="main">
        <TheCanvas />
    </main>
    <footer style="height: 60px"></footer>
</template>

<style>
* {
    box-sizing: border-box;
}

body {
    margin: 0;

    --color-text: hsl(34deg 57% 70%);
    --color-tool: hsl(34deg 24% 40%);
    --color-bg: hsl(32deg 23% 16%);

    /* --test-text: hsl(245deg 100% 55%);
    --test-tool: hsl(245deg 100% 40%);
    --test-bg: hsl(245deg 100% 30%);

    --color-text: var(--test-text);
    --color-tool: var(--test-tool);
    --color-bg: var(--test-bg); */

    background-color: var(--color-bg);
    color: var(--color-text);
}

input {
    display: inline-block;
}

#app {
    padding: 16px;
    width: 100vw;
    height: 100vh;
    display: flex;
    flex-direction: column;
}

.toolbar {
    display: flex;
    flex-direction: column;
}

.toolbar > * {
    border-bottom: 6px solid var(--color-tool);
}

.toolbar__line-1 {
    display: flex;
    align-items: center;
    justify-content: space-between;
}

.main {
    flex-grow: 1;
    display: flex;
    align-items: center;
    justify-content: center;
}
</style>
