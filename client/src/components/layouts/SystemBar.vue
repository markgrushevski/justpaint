<script setup lang="ts"></script>

<template>
    <div class="system-bar"><slot></slot></div>
</template>

<style>
.system-bar {
    grid-area: system-bar;

    display: grid;
    grid-template-columns: auto min-content min-content;
    gap: 20px;
}
</style>
