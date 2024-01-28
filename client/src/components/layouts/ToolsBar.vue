<script setup lang="ts"></script>

<template>
    <div class="tools-bar"><slot></slot></div>
</template>

<style>
.tools-bar {
    grid-area: tools-bar;
    display: flex;
    flex-direction: column;
}
</style>
