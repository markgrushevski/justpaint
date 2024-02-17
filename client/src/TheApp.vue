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
        <div class="time">{{ new Date(Date.now()).getHours() }}:{{ new Date(Date.now()).getMinutes() }}</div>
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
    font-family: 'Nunito', sans-serif;
    font-optical-sizing: auto;
    font-weight: 500;
    font-style: normal;
}

html,
body {
    font-size: 16px;
}

input {
    font-size: 1em;
}

:root {
    --color-backdrop: #f6f6f7;

    --color-accent_active: #76a573;
    --color-text_active: #000000;
    --color-background_active: #ffffff;

    --color-accent: #9bbd98;
    --color-text: #202020;
    --color-background: #f6f6f7;

    /*--cambridge-blue: #9abd97;
    --celadon: #b6d7b9;
    --dark-moss-green: #646536;
    --tea-green: #d0f1bf;*/

    --border-width: 2px;
    --border-radius: 0;
}

body {
    margin: 0;

    width: 100vw;
    height: 100vh;

    background-color: var(--color-backdrop);
    color: var(--color-text);
}

#app {
    width: 100%;
    height: 100%;

    display: grid;
    grid-template-rows: 40px 60px auto 40px;
    grid-template-columns: 60px 60px auto;
    grid-template-areas:
        'system-bar system-bar system-bar'
        'work-bar work-bar work-bar'
        'tools-bar settings-bar work-area'
        'status-bar status-bar status-bar';
}

.system-bar,
.work-bar,
.status-bar {
    padding: 8px;
}

.tools-bar,
.settings-bar {
    padding: 8px;
}

.work-bar {
    border-block: var(--border-width) solid var(--color-accent);
}
</style>
