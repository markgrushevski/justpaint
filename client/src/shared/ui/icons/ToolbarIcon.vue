<script setup lang="ts">
import WrapperIcon from './WrapperIcon.vue';

defineProps<{ iconPathValue: string; isActive?: boolean }>();
</script>

<template>
    <WrapperIcon class="toolbar__icon" :class="{ toolbar__icon_active: isActive }" width="2.5rem" height="2.5rem">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
            <path :d="iconPathValue" />
        </svg>
    </WrapperIcon>
</template>

<style>
.toolbar__icon > * {
    cursor: pointer;
    fill: var(--color-accent);

    /* mask-size: cover;
    mask-image: url(/pen.svg);
    background-color: var(--color-accent); */
}

.toolbar__icon_active > *,
.toolbar__icon:hover > * {
    fill: var(--color-accent_active);

    /* background-color: var(--color-text); */
}
</style>
