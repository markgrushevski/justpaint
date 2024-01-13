<script setup lang="ts">
defineProps<{ isActive?: boolean }>();
</script>

<template>
    <div class="work-handlers__handler" :class="{ 'work-handlers__handler_active': isActive }">
        <slot />
    </div>
</template>

<style>
.work-handlers__handler {
    width: 1.75em;
    height: 1.75em;
}

.work-handlers__handler > * {
    width: 100%;
    height: 100%;
    fill: var(--color-tool);
    cursor: pointer;
}

.work-handlers__handler:hover > *,
.work-handlers__handler_active > * {
    fill: var(--color-text);
}
</style>
