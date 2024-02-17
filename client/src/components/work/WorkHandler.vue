<script setup lang="ts">
defineProps<{ iconPathValue: string; isActive?: boolean }>();
</script>

<template>
    <div class="work-handlers__handler" :class="{ 'work-handlers__handler_active': isActive }">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
            <path :d="iconPathValue" />
        </svg>
    </div>
</template>

<style>
.work-handlers__handler {
    width: 1.75rem;
    height: 1.75rem;
}

.work-handlers__handler > * {
    width: 100%;
    height: 100%;
    fill: var(--color-accent);
    cursor: pointer;
}

.work-handlers__handler_active > *,
.work-handlers__handler:hover > * {
    fill: var(--color-accent_active);
}
</style>
