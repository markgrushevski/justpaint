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
} from '@components'
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
    --color-backdrop: hsl(110, 1%, 15%);

    --color-accent_active: hsl(110, 5%, 40%);
    --color-text_active: #ffffff;
    --color-borders_active: hsl(110, 5%, 40%);
    --color-background_active: hsl(110, 1%, 25%);

    --color-accent: hsl(110, 5%, 30%);
    --color-text: #f0f0f0;
    --color-borders: hsl(110, 5%, 30%);
    --color-background: hsl(110, 1%, 20%);

    /*--color-backdrop: #ffffff;

    --color-accent_active: #10c0ff;
    --color-text_active: #10c0ff;
    --color-borders_active: #f2a279;
    --color-background_active: #f0f0f0;

    --color-accent: #f2a279;
    --color-text: #f2a279;
    --color-borders: #f2a279;
    --color-background: #ffffff;*/

    --color-1: #373f51;
    --color-2: #1d201f;
    --color-3: #80d4ff;
    --color-4: #7bc950;
    --color-5: #f2a279;
    --color-6: #b84d18;

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
    border-block: var(--border-width) solid var(--color-borders);
}

.tools-bar {
    padding: 8px;
    border-inline-start: var(--border-width) solid var(--color-borders);
}

.work-area {
}

.settings-bar {
    padding: 10px 8px;
    border-inline-end: var(--border-width) solid var(--color-borders);
}

.status-bar {
    padding: 8px 10px;
    border-block-start: var(--border-width) solid var(--color-borders);
}
</style>
