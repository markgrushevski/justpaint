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
    --color-backdrop: rgb(46, 52, 64);

    --color-accent_active: rgb(136, 192, 208);
    --color-text_active: rgb(216, 222, 233);
    --color-background_active: rgb(76, 86, 106);

    --color-accent: rgb(97, 110, 136);
    --color-text: rgb(216, 222, 233);
    --color-background: rgb(59, 66, 82);

    /*--color-backdrop: #f6f6f7;*/

    /*--color-accent_active: #76a573;
    --color-text_active: #000000;
    --color-background_active: #ffffff;*/

    /*--color-accent: #9bbd98;
    --color-text: #202020;
    --color-background: #f6f6f7;*/

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
    grid-template-columns: 3.5em 4em auto;
    grid-template-areas:
        'system-bar system-bar system-bar'
        'work-bar work-bar work-bar'
        'tools-bar settings-bar work-area'
        'status-bar status-bar status-bar';
}

.system-bar,
.status-bar {
    padding: 8px 10px;
}

.work-bar,
.tools-bar {
    padding: 8px;
}

.settings-bar {
    padding: 10px 8px;
}

.work-bar {
    border-bottom: var(--border-width) dashed var(--color-accent);
}
</style>
