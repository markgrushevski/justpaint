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
        <div class="system-bar__work-title">My Work 1</div>
        <div class="system-bar__weather">0</div>
        <div class="system-bar__time">
            {{ new Date(Date.now()).getHours() }}:{{ new Date(Date.now()).getMinutes() }}
        </div>
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
    /*--color-backdrop: hsl(220, 16%, 22%);

    --color-accent_active: hsl(193, 43%, 67%);
    --color-text_active: hsl(219, 28%, 88%);
    --color-background_active: hsl(220, 16%, 36%);

    --color-accent: hsl(220, 17%, 46%);
    --color-text: hsl(219, 28%, 88%);
    --color-background: hsl(222, 16%, 28%);*/

    --color-backdrop: hsl(110, 1%, 15%);

    --color-accent_active: hsl(110, 5%, 40%);
    --color-text_active: #ffffff;
    --color-background_active: hsl(110, 1%, 25%);

    --color-accent: hsl(110, 5%, 30%);
    --color-text: #f0f0f0;
    --color-background: hsl(110, 1%, 20%);

    /*--color-backdrop: #282a2c;

    --color-accent_active: #b0b8c0;
    --color-text_active: #ffffff;
    --color-background_active: #303234;

    --color-accent: #808890;
    --color-text: #f0f0f0;
    --color-background: #282a2c;*/

    --border-width: 1px;
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
    grid-template-rows: 2.5em 2.5em auto 2em;
    grid-template-columns: min-content auto 3.5em;
    grid-template-areas:
        'system-bar system-bar system-bar'
        'work-bar work-bar work-bar'
        'settings-bar work-area tools-bar'
        'status-bar status-bar status-bar';
}

.system-bar {
    padding: 8px 10px;
}

.work-bar {
    padding: 8px;
    border-block: var(--border-width) solid var(--color-accent);
}

.tools-bar {
    padding: 8px;
    border-inline-start: var(--border-width) solid var(--color-accent);
}

.work-area {
}

.settings-bar {
    padding: 10px 8px;
    border-inline-end: var(--border-width) solid var(--color-accent);
}

.status-bar {
    padding: 8px 10px;
    border-block-start: var(--border-width) solid var(--color-accent);
}
</style>
