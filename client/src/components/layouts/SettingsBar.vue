<script setup lang="ts"></script>

<template>
    <div class="settings-bar"><slot></slot></div>
</template>

<style>
.settings-bar {
    grid-area: settings-bar;

    display: flex;
    align-items: center;
    justify-content: flex-start;
    flex-direction: column;

    gap: 1em;

    border-inline: var(--border-width) solid var(--color-accent);

    font-size: 14px;

    text-align: center;
}

.settings-bar input {
    border: 0;

    border-radius: var(--border-radius);

    color: var(--color-text);
    outline-color: var(--color-accent);
    outline-width: 2px;
    background-color: var(--color-background);
}

.settings-bar input[type='color'] {
    width: 2.5em;
    height: 2.5em;
}

.settings-bar input[type='number'] {
    padding: 3px 7px;
    width: 3em;
}

.settings-bar input:focus-visible,
.settings-bar input:focus-within,
.settings-bar input:focus {
    outline-style: solid;
}

.settings-bar input[type='number']:focus-visible,
.settings-bar input[type='number']:focus-within,
.settings-bar input[type='number']:focus {
    min-width:;
}
</style>
