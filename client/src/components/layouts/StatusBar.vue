<script setup lang="ts"></script>

<template>
    <div class="status-bar"><slot></slot></div>
</template>

<style>
.status-bar {
    grid-area: status-bar;

    display: flex;
    align-items: center;
    justify-content: flex-end;

    border-block-start: var(--border-width) solid var(--color-accent);
}
</style>
