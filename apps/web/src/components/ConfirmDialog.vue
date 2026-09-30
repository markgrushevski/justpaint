<script lang="ts" setup>
/**
 * Generic confirm modal on OriDialog's controlled form; the native <dialog>
 * gives focus trap, scroll lock, Esc and ::backdrop dismissal for free. The
 * parent owns `open` and never mutates it — controlled mode is optimistic, so
 * a user dismiss has already closed the dialog by the time `update:open(false)`
 * fires, and `onOpenChange` just maps that to `cancel`. /draw's confirms and every
 * editor view's leave question use it.
 */
import { OriButton, OriDialog } from '@oriui/vue'

const props = defineProps<{
    open: boolean
    title: string
    message?: string
    confirmText?: string
    cancelText?: string
    danger?: boolean
}>()

const emit = defineEmits<{ confirm: []; cancel: [] }>()

function onOpenChange(open: boolean) {
    if (!open) emit('cancel')
}
</script>

<template>
    <OriDialog :open="props.open" modal :title="props.title" @update:open="onOpenChange">
        <p v-if="props.message" class="confirm__message">{{ props.message }}</p>

        <div class="confirm__actions">
            <OriButton :label="props.cancelText ?? 'Cancel'" variant="outline" radius="md" @click="emit('cancel')" />
            <OriButton
                :label="props.confirmText ?? 'Confirm'"
                variant="solid"
                :color="props.danger ? 'danger' : undefined"
                radius="md"
                @click="emit('confirm')"
            />
        </div>
    </OriDialog>
</template>

<style scoped>
.confirm__message {
    margin: 0;
    font-size: var(--ori-font-size_sm, 0.875rem);
    line-height: 1.5;
    opacity: 0.85;
}

.confirm__actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--ori-size-gap_sm, 0.25rem);

    margin-top: var(--ori-size-gap_sm, 0.25rem);
}
</style>
