<script lang="ts" setup>
/**
 * The keyboard-shortcuts cheat-sheet (DECISIONS 2026-07-04): a centered modal
 * floating island, opened with "?" or the top-right help chip. Hand-rolled on
 * the SideMenu pattern (Teleport + Transition + focus-on-open + Esc) because
 * @oriui/vue's OriDialog (alpha-2) is uncontrolled — `defaultOpen` + a trigger
 * slot, no `open` prop / `close` emit — so it can't be driven by the host
 * view's `shortcutsOpen` ref. Tool rows come from TOOL_META (the single hint
 * source shared with the toolbar tooltips).
 */
import { nextTick, ref, watch } from 'vue'
import { OriKbd } from '@oriui/vue'
import { TOOLS } from '@justpaint/editor'
import type { ToolId } from '@justpaint/editor'
import { TOOL_META } from './FloatingToolbar.vue'
import ToolIcon from './icons/ToolIcon.vue'

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

// Move focus into the dialog on open (the SideMenu pattern) — without focus
// inside, the panel-tree Esc handler below would never fire.
const panelRef = ref<HTMLElement | null>(null)
watch(
    () => props.open,
    async (open) => {
        if (open) {
            await nextTick()
            panelRef.value?.focus()
        }
    }
)

function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') emit('close')
}
</script>

<template>
    <Teleport to="body">
        <!-- Explicit duration: Vue then uses a timer, not transitionend — deterministic in hidden tabs/tests. -->
        <Transition name="shortcuts" :duration="200">
            <div v-if="props.open" class="shortcuts" @keydown="onKeydown">
                <div class="shortcuts__backdrop" aria-hidden="true" @click="emit('close')"></div>

                <div
                    ref="panelRef"
                    class="shortcuts__panel jp-float"
                    role="dialog"
                    aria-modal="true"
                    aria-label="Keyboard shortcuts"
                    tabindex="-1"
                >
                    <header class="shortcuts__head">
                        <h2 class="shortcuts__title">Keyboard shortcuts</h2>
                        <button class="shortcuts__close" type="button" aria-label="Close" @click="emit('close')">
                            <ToolIcon name="close" />
                        </button>
                    </header>

                    <section
                        v-for="group in GROUPS"
                        :key="group.title"
                        class="shortcuts__group"
                        :aria-label="group.title"
                    >
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
                </div>
            </div>
        </Transition>
    </Teleport>
</template>

<style scoped>
.shortcuts {
    position: fixed;
    inset: 0;
    z-index: 100;

    display: grid;
    place-items: center;

    padding: var(--ori-size-gap_lg, 0.75rem);
}

.shortcuts__backdrop {
    position: absolute;
    inset: 0;
    background-color: rgb(0 0 0 / 35%);
}

.shortcuts__panel {
    position: relative;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);

    width: 100%;
    max-width: 26rem;
    max-height: 100%;
    padding: var(--ori-size-gap_lg, 0.75rem);
    overflow-y: auto;

    color: var(--ori-color-on-surface);
}

.shortcuts__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
}

.shortcuts__title {
    margin: 0;
    font-size: var(--ori-font-size_lg, 1.15rem);
    font-weight: 700;
    letter-spacing: -0.01em;
}

.shortcuts__close {
    display: grid;
    place-items: center;

    width: 2.2rem;
    height: 2.2rem;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    cursor: pointer;
}

.shortcuts__close:hover {
    background-color: color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent);
}

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

/* Fade + gentle rise (the toast/menu language, centered). */
.shortcuts-enter-active,
.shortcuts-leave-active {
    transition: opacity 180ms ease;
}

.shortcuts-enter-active .shortcuts__panel,
.shortcuts-leave-active .shortcuts__panel {
    transition: transform 180ms ease;
}

.shortcuts-enter-from,
.shortcuts-leave-to {
    opacity: 0;
}

.shortcuts-enter-from .shortcuts__panel,
.shortcuts-leave-to .shortcuts__panel {
    transform: translateY(0.4rem) scale(0.98);
}
</style>
