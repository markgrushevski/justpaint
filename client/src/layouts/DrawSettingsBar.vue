<script setup lang="ts"></script>

<template>
    <div class="draw-settings-bar"><slot></slot></div>
</template>

<style>
.draw-settings-bar {
    grid-area: draw-settings-bar;

    display: flex;
    align-items: center;
    justify-content: flex-start;
    flex-direction: column;

    gap: 1em;

    font-size: 14px;

    text-align: center;
}
</style>
