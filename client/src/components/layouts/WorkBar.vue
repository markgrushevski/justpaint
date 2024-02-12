<script setup lang="ts"></script>

<template>
    <div class="work-bar"><slot></slot></div>
</template>

<style>
.work-bar {
    grid-area: work-bar;

    display: flex;
    align-items: center;
    justify-content: end;
}
</style>
