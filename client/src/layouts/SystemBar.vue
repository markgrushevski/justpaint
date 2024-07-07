<script setup lang="ts"></script>

<template>
    <div class="system-bar"><slot></slot></div>
</template>

<style>
.system-bar {
    grid-area: system-bar;

    display: flex;
    align-items: center;
    gap: 20px;
}
</style>
