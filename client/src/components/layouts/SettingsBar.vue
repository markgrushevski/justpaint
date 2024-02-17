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
    width: 40px;
    height: 40px;
}

.settings-bar input[type='number'] {
    padding: 3px 7px;
    width: 50px;
}

.settings-bar input:focus-visible,
.settings-bar input:focus-within,
.settings-bar input:focus {
    outline-style: solid;
}
</style>
