<script setup lang="ts">
import { DrawSettingsBar, SystemBar, StatusBar, WorkBar, WorkArea, DrawToolsBar } from '@layouts'
import { DrawTools, WorkHandlers, SystemSettings, DrawSettings, TheCanvas } from '@components'
</script>

<template>
    <SystemBar>
        <SystemSettings />
    </SystemBar>

    <WorkBar>
        <WorkHandlers />
    </WorkBar>

    <DrawToolsBar>
        <DrawTools />
    </DrawToolsBar>

    <WorkArea>
        <TheCanvas />
    </WorkArea>

    <DrawSettingsBar>
        <DrawSettings />
    </DrawSettingsBar>

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
    /* --color-backdrop: hsl(110, 1%, 15%);

    --color-accent_active: hsl(110, 5%, 40%);
    --color-text_active: #ffffff;
    --color-borders_active: hsl(110, 5%, 40%);
    --color-background_active: hsl(110, 1%, 25%);

    --color-accent: hsl(110, 5%, 30%);
    --color-text: #f0f0f0;
    --color-borders: hsl(110, 5%, 30%);
    --color-background: hsl(110, 1%, 20%); */

    /* --color-backdrop: #ffffff;

    --color-accent_active: #10c0ff;
    --color-text_active: #10c0ff;
    --color-borders_active: #f2a279;
    --color-background_active: #f0f0f0;

    --color-accent: #f2a279;
    --color-text: #f2a279;
    --color-borders: #f2a279;
    --color-background: #ffffff; */

    /* --color-backdrop: #ffffff;

    --color-accent_active: #60e0ff;
    --color-text_active: #20c8ff;
    --color-borders_active: #20c8ff;
    --color-background_active: #f0f0f0;

    --color-accent: #30d0ff;
    --color-text: #10c0ff;
    --color-borders: #10c0ff;
    --color-background: #ffffff; */

    --color-accent: hsl(110, 5%, 30%);
    --color-accent_active: hsl(110, 5%, 40%);

    --color-text: hsl(120, 3%, 75%);
    --color-text_active: hsl(0, 0%, 100%);

    --color-borders: hsl(110, 5%, 25%);
    --color-borders_active: hsl(110, 5%, 35%);

    --color-background: hsl(110, 1%, 20%);
    --color-background_active: hsl(110, 1%, 25%);

    --color-backdrop: hsl(110, 1%, 17%);

    --color-page: hsl(110, 1%, 15%);

    --color-1: #373f51;
    --color-2: #1d201f;
    --color-3: #80d4ff;
    --color-4: #7bc950;
    --color-5: #f2a279;
    --color-6: #b84d18;

    --border-width-blocks: 1px;

    --border-radius-blocks: 16px;
    --border-radius-controls: 0;
}

body {
    margin: 0;

    width: 100vw;
    height: 100vh;

    background-color: var(--color-page);
    color: var(--color-text);
}

#justpaint {
    width: 100%;
    height: 100%;

    display: grid;
    grid-template-rows: 3em 2.5em auto 2em;
    grid-template-columns: 6em auto 12em;
    grid-template-areas:
        'system-bar system-bar system-bar'
        'work-bar work-bar work-bar'
        'draw-tools-bar work-area draw-settings-bar'
        'status-bar status-bar status-bar';
}

/* top 1 */
.system-bar {
    padding: 10px 12px;

    /* background-color: var(--color-backdrop); */
}

/* top 2 */
.work-bar {
    padding: 8px;

    /* border-top: var(--border-width-blocks) solid var(--color-borders); */
}

/* left */
.draw-tools-bar {
    padding: 8px;
    height: min-content;

    background-color: var(--color-backdrop);

    /* border-inline-end: var(--border-width-blocks) solid var(--color-borders); */
    border-start-end-radius: var(--border-radius-blocks);
    border-end-end-radius: var(--border-radius-blocks);
}

/* center */
.work-area {
    /* background-color: var(--color-backdrop); */

    /* border-radius: var(--border-radius-blocks); */
}

/* right */
.draw-settings-bar {
    padding: 10px 8px 14px;
    height: min-content;

    background-color: var(--color-backdrop);

    /* border-inline-start: var(--border-width-blocks) solid var(--color-borders); */
    border-start-start-radius: var(--border-radius-blocks);
    border-end-start-radius: var(--border-radius-blocks);
}

/* bottom */
.status-bar {
    padding: 8px 12px;

    /* border-block-start: var(--border-width-blocks) solid var(--color-borders); */
}
</style>
