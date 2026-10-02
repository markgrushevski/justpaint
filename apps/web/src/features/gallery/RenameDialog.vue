<script lang="ts" setup>
/**
 * Rename modal on OriDialog's controlled form. The parent owns `open`; a dismiss has
 * already closed the box by the time `update:open(false)` fires, so it maps to `cancel`.
 */
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
import type { ComponentPublicInstance } from 'vue'
import { OriButton, OriDialog, OriInput } from '@oriui/vue'

/** The API caps a name at 64 runes. */
const MAX_NAME = 64

const props = defineProps<{ open: boolean; name: string; busy?: boolean }>()
const emit = defineEmits<{ save: [name: string]; cancel: [] }>()

const draft = ref(props.name)
const field = useTemplateRef<ComponentPublicInstance>('field')
const trimmed = computed(() => draft.value.trim())

// Opening starts from the current name, selected, so typing replaces it. The dialog
// focuses its close button first, so the input takes focus once the box is shown.
watch(
    () => props.open,
    async (open) => {
        if (!open) return
        draft.value = props.name
        await nextTick()
        const input = (field.value?.$el as HTMLElement | undefined)?.querySelector('input')
        input?.focus()
        input?.select()
    }
)

function onOpenChange(open: boolean): void {
    if (!open) emit('cancel')
}

function submit(): void {
    if (!trimmed.value || props.busy) return
    if (trimmed.value === props.name) emit('cancel')
    else emit('save', trimmed.value)
}

function onEnter(event: KeyboardEvent): void {
    if (!event.isComposing) submit()
}
</script>

<template>
    <OriDialog :open="props.open" modal title="Rename drawing" @update:open="onOpenChange">
        <OriInput
            ref="field"
            v-model="draft"
            label="Name"
            fluid
            :maxlength="MAX_NAME"
            :disabled="props.busy"
            autocomplete="off"
            @keydown.enter="onEnter"
        />

        <div class="rename__actions">
            <OriButton
                label="Cancel"
                variant="outline"
                color="surface"
                radius="md"
                :disabled="props.busy"
                @click="emit('cancel')"
            />
            <OriButton
                label="Save"
                variant="solid"
                radius="md"
                :disabled="!trimmed"
                :loading="props.busy"
                @click="submit"
            />
        </div>
    </OriDialog>
</template>

<style scoped>
.rename__actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--ori-size-gap_sm, 0.25rem);

    margin-top: var(--ori-size-gap_lg, 0.75rem);
}
</style>
