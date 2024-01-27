<script lang="ts" setup>
withDefaults(defineProps<{ width?: string; height?: string }>(), { width: '1.5rem', height: '1.5rem' });
</script>

<template>
    <div class="wrapper-icon">
        <slot />
    </div>
</template>

<style>
.wrapper-icon {
    display: inline-block;
    width: v-bind(width);
    height: v-bind(height);
}

.wrapper-icon > * {
    width: 100%;
    height: 100%;
}
</style>
