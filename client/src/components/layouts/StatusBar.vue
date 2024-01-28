<script setup lang="ts"></script>

<template>
    <div class="status-bar"><slot></slot></div>
</template>

<style>
.status-bar {
    grid-area: status-bar;

    display: flex;
}
</style>
