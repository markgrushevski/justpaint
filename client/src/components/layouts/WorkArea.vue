<script setup lang="ts"></script>

<template>
    <main class="work-area"><slot></slot></main>
</template>

<style>
.work-area {
    grid-area: work-area;

    display: flex;
}
</style>
