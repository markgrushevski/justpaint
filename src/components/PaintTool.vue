<script setup lang="ts">
defineProps<{ isActive?: boolean }>();
</script>

<template>
    <div class="paint-tools__tool" :class="{ 'paint-tools__tool_active': isActive }">
        <slot />
    </div>
</template>

<style>
.paint-tools__tool {
    width: 2.5em;
    height: 2.5em;
}

.paint-tools__tool > * {
    width: 100%;
    height: 100%;

    border: 0;

    border-radius: 1px;

    fill: var(--color-tool);
    background-color: transparent;
    cursor: pointer;
}

.paint-tools__tool:hover > *,
.paint-tools__tool_active > * {
    fill: var(--color-text);
}
</style>
