<script lang="ts" setup>
/**
 * Names a drawing on its first save. Later saves keep the name; the gallery renames.
 * Controlled like ConfirmDialog: the parent owns `open`, a dismiss reports `cancel`.
 */
import { nextTick, ref, watch } from 'vue'
import { OriButton, OriDialog, OriInput } from '@oriui/vue'

const NAME_MAX = 64

const props = defineProps<{ open: boolean; initial: string }>()
const emit = defineEmits<{ save: [name: string]; cancel: [] }>()

const name = ref(props.initial)
const fieldRef = ref<HTMLElement | null>(null)

// Refill on every open and select the text, so typing replaces the placeholder name.
watch(
    () => props.open,
    async (open) => {
        if (!open) return
        name.value = props.initial
        await nextTick()
        const input = fieldRef.value?.querySelector('input')
        input?.focus()
        input?.select()
    }
)

function submit() {
    const next = name.value.trim().slice(0, NAME_MAX)
    if (next) emit('save', next)
}

// Enter that commits an IME composition is not a submit.
function onEnter(event: KeyboardEvent) {
    if (!event.isComposing) submit()
}

function onOpenChange(open: boolean) {
    if (!open) emit('cancel')
}
</script>

<template>
    <OriDialog :open="props.open" modal title="Save drawing" @update:open="onOpenChange">
        <div ref="fieldRef">
            <OriInput v-model="name" label="Name" :maxlength="NAME_MAX" fluid @keydown.enter="onEnter" />
        </div>

        <div class="save__actions">
            <OriButton label="Cancel" variant="outline" radius="md" @click="emit('cancel')" />
            <OriButton label="Save" variant="solid" radius="md" :disabled="!name.trim()" @click="submit" />
        </div>
    </OriDialog>
</template>

<style scoped>
.save__actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--ori-size-gap_sm, 0.25rem);

    margin-top: var(--ori-size-gap_md, 0.5rem);
}
</style>
