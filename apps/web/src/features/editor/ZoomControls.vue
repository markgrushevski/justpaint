<script lang="ts" setup>
/** The zoom island every canvas route puts in its bottom-right corner. */
import { OriSurface } from '@oriui/vue'
import IconButton from '../../components/ui/IconButton.vue'

defineProps<{ percent: number }>()
const emit = defineEmits<{ zoomIn: []; zoomOut: []; fit: [] }>()
</script>

<template>
    <OriSurface class="zoom" role="group" aria-label="Zoom">
        <IconButton icon="minus" label="Zoom out — Ctrl+-" @click="emit('zoomOut')" />
        <span class="zoom__value">{{ percent }}%</span>
        <IconButton icon="plus" label="Zoom in — Ctrl+=" @click="emit('zoomIn')" />
        <IconButton icon="fit" label="Fit — Ctrl+0" @click="emit('fit')" />
    </OriSurface>
</template>

<style scoped>
.zoom {
    display: flex;
    align-items: center;
    gap: 0;

    padding: var(--ori-size-gap_xs, 0.125rem) var(--ori-size-gap_sm, 0.25rem);
}

.zoom__value {
    min-width: 3.1rem;
    padding: 0.25rem;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-variant-numeric: tabular-nums;
    text-align: center;
}
</style>
