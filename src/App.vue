<script setup lang="ts">
import { PaintTools, TheCanvas, ToolsSettings, WorkHandlers } from '@components';
</script>

<template>
    <header class="header">
        <div class="header__toolbar">
            <PaintTools />
            <WorkHandlers />
        </div>

        <div class="header__settings">
            <ToolsSettings />
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

    --color-text: #deb887;
    --color-tool: #806a4e;
    --color-bg: #332a20;
    background-color: var(--color-bg);
    color: var(--color-text);
}

#app {
    padding: 16px;

    width: 100vw;
    height: 100vh;

    display: flex;
    flex-direction: column;
}

.header {
    display: flex;
    flex-direction: column;
}

.header > * {
    border-bottom: 6px solid var(--color-tool);
}

.header__toolbar {
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
