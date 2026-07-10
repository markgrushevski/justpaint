<script lang="ts" setup>
/**
 * The keyboard-shortcuts cheat-sheet (DECISIONS 2026-07-04): a centered modal
 * floating island, opened with "?" or the top-right help chip. Built on
 * @oriui/vue's OriDialog (the alpha-11 controlled form: `open` prop +
 * `update:open`/`close` emits) — the native <dialog> supplies the focus trap,
 * scroll lock, Esc and ::backdrop dismissal for free, and its own header
 * renders the title + × close button. Tool rows come from TOOL_META (the
 * single hint source shared with the toolbar tooltips).
 */
import { OriDialog, OriKbd } from '@oriui/vue'
import { TOOLS } from '@justpaint/editor'
import type { ToolId } from '@justpaint/editor'
import { TOOL_META } from './FloatingToolbar.vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

interface ShortcutRow {
    /** Alternative key combos for one action, rendered "A / B". */
    keys: string[]
    action: string
}

const GROUPS: { title: string; rows: ShortcutRow[] }[] = [
    {
        title: 'Tools',
        rows: (Object.keys(TOOLS) as ToolId[]).map((id) => ({
            keys: [TOOL_META[id].key],
            action: TOOL_META[id].label
        }))
    },
    {
        title: 'History',
        rows: [
            { keys: ['Ctrl+Z'], action: 'Undo' },
            { keys: ['Ctrl+Shift+Z', 'Ctrl+Y'], action: 'Redo' }
        ]
    },
    {
        title: 'View',
        rows: [
            { keys: ['Ctrl+0'], action: 'Fit to view' },
            { keys: ['Ctrl+='], action: 'Zoom in' },
            { keys: ['Ctrl+-'], action: 'Zoom out' }
        ]
    },
    {
        title: 'File',
        rows: [{ keys: ['Ctrl+S'], action: 'Save' }]
    }
]

function onOpenChange(open: boolean) {
    if (!open) emit('close')
}
</script>

<template>
    <OriDialog :open="props.open" modal title="Keyboard shortcuts" @update:open="onOpenChange">
        <section v-for="group in GROUPS" :key="group.title" class="shortcuts__group" :aria-label="group.title">
            <h3 class="shortcuts__group-title">{{ group.title }}</h3>
            <ul class="shortcuts__rows">
                <li v-for="row in group.rows" :key="row.action" class="shortcuts__row">
                    <span class="shortcuts__keys">
                        <template v-for="(combo, i) in row.keys" :key="combo">
                            <span v-if="i > 0" class="shortcuts__or" aria-hidden="true">/</span>
                            <OriKbd :text="combo" />
                        </template>
                    </span>
                    <span class="shortcuts__action">{{ row.action }}</span>
                </li>
            </ul>
        </section>
    </OriDialog>
</template>

<style scoped>
.shortcuts__group {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.shortcuts__group-title {
    margin: 0;

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    opacity: 0.6;
}

.shortcuts__rows {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);

    margin: 0;
    padding: 0;
    list-style: none;
}

/* Two columns: kbd chips left (fixed rail so actions align), action right. */
.shortcuts__row {
    display: grid;
    grid-template-columns: 9.5rem 1fr;
    column-gap: var(--ori-size-gap_md, 0.5rem);
    align-items: center;
}

.shortcuts__keys {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
    font-size: var(--ori-font-size_md, 1rem);
}

.shortcuts__or {
    font-size: var(--ori-font-size_xs, 0.75rem);
    opacity: 0.6;
}

.shortcuts__action {
    font-size: var(--ori-font-size_sm, 0.875rem);
}
</style>
