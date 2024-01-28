<script setup lang="ts"></script>

<template>
    <div class="settings-bar"><slot></slot></div>
</template>

<style>
.settings-bar {
    grid-area: settings-bar;
    display: flex;
    flex-direction: column;
}
</style>
