<script setup lang="ts">
import WrapperIcon from './WrapperIcon.vue'

withDefaults(
    defineProps<{
        width?: string
        height?: string
        iconPathValue?: string
        isActive?: boolean
    }>(),
    {
        width: '2rem',
        height: '2rem'
    }
)
</script>

<template>
    <WrapperIcon :width="width" :height="height" :class="{ ['action-icon_active']: isActive }" class="action-icon">
        <svg v-if="iconPathValue" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
            <path :d="iconPathValue" />
        </svg>

        <slot></slot>
    </WrapperIcon>
</template>

<style>
.action-icon > * {
    cursor: pointer;
    fill: var(--color-accent);
}

.action-icon_active > *,
.action-icon:hover > * {
    fill: var(--color-accent_active);
}
</style>
