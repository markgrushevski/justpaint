<script lang="ts" setup>
/**
 * A generic, product-agnostic confirmation modal — "are you sure?" before a
 * destructive or irreversible action. Built on @oriui/vue's OriDialog (the
 * alpha-11 controlled form: `open` prop + `update:open`/`close` emits) — the
 * native <dialog> supplies the focus trap, scroll lock, Esc and ::backdrop
 * dismissal for free.
 *
 * The parent owns `open` and reacts to `confirm` / `cancel`; this component
 * never mutates the prop. A user-initiated dismiss (Esc / backdrop / ×) maps
 * to `cancel` — controlled mode is optimistic, so the dialog has already
 * closed by the time `update:open(false)` fires; we just mirror it outward.
 * Reused by /draw today and the /play game shell later, so it stays free of
 * any product-specific copy or wiring.
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
            <OriButton :text="props.cancelText ?? 'Cancel'" variant="outline" radius="md" @click="emit('cancel')" />
            <OriButton
                :text="props.confirmText ?? 'Confirm'"
                variant="fill"
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

/* Cancel then Confirm, right-aligned. */
.confirm__actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--ori-size-gap_sm, 0.25rem);

    margin-top: var(--ori-size-gap_sm, 0.25rem);
}
</style>
