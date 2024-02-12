<script setup lang="ts">
import WorkHandler from './WorkHandler.vue';
</script>

<template>
    <WorkHandler title="Undo" type="undo" />
    <WorkHandler title="Redo" type="redo" />
    <WorkHandler title="Save" type="save" />
</template>
