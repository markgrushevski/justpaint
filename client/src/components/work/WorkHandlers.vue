<script setup lang="ts">
import WorkHandler from './WorkHandler.vue';
</script>

<template>
    <div class="work-handlers">
        <WorkHandler title="Undo" type="undo" />
        <WorkHandler title="Redo" type="redo" />
        <WorkHandler title="Save" type="save" />
    </div>
</template>

<style>
.work-handlers {
    display: flex;
    align-items: center;
    gap: 4px;
}
</style>
