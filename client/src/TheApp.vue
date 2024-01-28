<script setup lang="ts">
import {
    PaintTools,
    PaintToolSettings,
    SettingsBar,
    TheCanvas,
    WorkHandlers,
    SystemBar,
    StatusBar,
    WorkBar,
    WorkArea,
    ToolsBar
} from '@components';
</script>

<template>
    <SystemBar>
        <div class="title">New Project</div>
        <div class="weather">0</div>
        <div class="time">{{ Date.now() }}</div>
    </SystemBar>

    <WorkBar>
        <WorkHandlers />
    </WorkBar>

    <ToolsBar>
        <PaintTools />
    </ToolsBar>

    <SettingsBar>
        <PaintToolSettings />
    </SettingsBar>

    <WorkArea>
        <TheCanvas />
    </WorkArea>

    <StatusBar>
        <div class="notifications">Notifications: ...</div>
    </StatusBar>
</template>

<style>
* {
    box-sizing: border-box;
}

body {
    margin: 0;

    --color-text: hsl(200deg 60% 70%);
    --color-tool: hsl(200deg 35% 40%);
    --color-bg: hsl(200deg 30% 15%);

    /*--test-text: #e4ecef;
    --test-tool: #ed7e5a;
    --test-bg: #fdfeff;

    --color-text: var(--test-text);
    --color-tool: var(--test-tool);
    --color-bg: var(--test-bg);*/

    background-color: var(--color-bg);
    color: var(--color-text);
}

#app {
    width: 100vw;
    height: 100vh;
    display: grid;
    grid-template-rows: 1.5em 2em auto 1.5em;
    grid-template-columns: 2em 2em auto;
    grid-template-areas:
        'system-bar system-bar system-bar'
        'work-bar work-bar work-bar'
        'tools-bar settings-bar work-area'
        'status-bar status-bar status-bar';
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
