<script lang="ts">
import type { ToolId } from '@justpaint/editor'
import type { IconName } from './icons/ToolIcon.vue'

/**
 * Label + hotkey per tool — the SINGLE source of hotkey hint text. The toolbar
 * tooltips, the DrawView key bindings, and the shortcuts cheat-sheet all read
 * from here, so a remapped key can never drift out of sync with its hint.
 */
export const TOOL_META: Record<ToolId, { label: string; icon: IconName; key: string }> = {
    pen: { label: 'Pen', icon: 'pen', key: 'B' },
    eraser: { label: 'Eraser', icon: 'eraser', key: 'E' },
    line: { label: 'Line', icon: 'line', key: 'L' },
    rect: { label: 'Rectangle', icon: 'rect', key: 'R' },
    ellipse: { label: 'Ellipse', icon: 'ellipse', key: 'O' },
    triangle: { label: 'Triangle', icon: 'triangle', key: 'T' }
}
</script>

<script lang="ts" setup>
/**
 * The floating bottom toolbar (tldraw-style — DECISIONS 2026-07-04): tools as
 * icon buttons, stroke/fill controls, undo/redo. Part of the shared /draw+/play
 * editor shell; file actions and panels live in the top clusters, not here.
 */
import { OriCheckbox, OriSlider } from '@oriui/vue'
import { TOOLS } from '@justpaint/editor'
import ToolIcon from './icons/ToolIcon.vue'

const toolIds = Object.keys(TOOLS) as ToolId[]

const props = defineProps<{
    activeTool: ToolId
    color: string
    strokeWidth: number
    fillEnabled: boolean
    fill: string
    canUndo: boolean
    canRedo: boolean
}>()

const emit = defineEmits<{
    pickTool: [id: ToolId]
    setColor: [hex: string]
    setWidth: [width: number]
    toggleFill: [enabled: boolean]
    setFill: [hex: string]
    undo: []
    redo: []
}>()

function onColor(e: Event) {
    emit('setColor', (e.target as HTMLInputElement).value)
}
function onFill(e: Event) {
    emit('setFill', (e.target as HTMLInputElement).value)
}
</script>

<template>
    <div class="bar jp-float" role="toolbar" aria-label="Drawing tools">
        <div class="bar__group" role="group" aria-label="Tools">
            <button
                v-for="id in toolIds"
                :key="id"
                class="bar__tool"
                :class="{ 'bar__tool--active': props.activeTool === id }"
                :aria-pressed="props.activeTool === id"
                :aria-label="TOOL_META[id].label"
                :title="`${TOOL_META[id].label} — ${TOOL_META[id].key}`"
                type="button"
                @click="emit('pickTool', id)"
            >
                <ToolIcon :name="TOOL_META[id].icon" />
            </button>
        </div>

        <span class="bar__divider" aria-hidden="true"></span>

        <div class="bar__group" role="group" aria-label="Stroke and fill">
            <label class="bar__swatch" title="Stroke color">
                <input type="color" :value="props.color" aria-label="Stroke color" @input="onColor" />
            </label>

            <div class="bar__slider" :title="`Width — ${props.strokeWidth}`">
                <OriSlider
                    :model-value="props.strokeWidth"
                    :min="1"
                    :max="64"
                    :step="1"
                    aria-label="Stroke width"
                    @update:model-value="(v: number) => emit('setWidth', v)"
                />
                <span class="bar__slider-value">{{ props.strokeWidth }}</span>
            </div>

            <div class="bar__fill">
                <OriCheckbox
                    :model-value="props.fillEnabled"
                    label="Fill"
                    @update:model-value="(v) => emit('toggleFill', v === true)"
                />
                <label class="bar__swatch" title="Fill color">
                    <input
                        type="color"
                        :value="props.fill"
                        :disabled="!props.fillEnabled"
                        aria-label="Fill color"
                        @input="onFill"
                    />
                </label>
            </div>
        </div>

        <span class="bar__divider" aria-hidden="true"></span>

        <div class="bar__group" role="group" aria-label="History">
            <button
                class="bar__tool"
                :disabled="!props.canUndo"
                aria-label="Undo"
                title="Undo — Ctrl/⌘+Z"
                type="button"
                @click="emit('undo')"
            >
                <ToolIcon name="undo" />
            </button>
            <button
                class="bar__tool"
                :disabled="!props.canRedo"
                aria-label="Redo"
                title="Redo — Ctrl/⌘+Y"
                type="button"
                @click="emit('redo')"
            >
                <ToolIcon name="redo" />
            </button>
        </div>
    </div>
</template>

<style scoped>
.bar {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);

    /* Never wider than the viewport; inner groups scroll-free compact on phones. */
    max-width: calc(100vw - 1rem);
}

.bar__group {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.bar__divider {
    align-self: stretch;
    width: 1px;
    margin: 0.2rem 0.15rem;
    background-color: var(--ori-color-outline, rgb(0 0 0 / 12%));
}

.bar__tool {
    display: grid;
    place-items: center;

    width: var(--jp-control-lg, 2.4rem);
    height: var(--jp-control-lg, 2.4rem);
    padding: 0;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    font-size: 1rem;
    cursor: pointer;
    transition:
        background-color 120ms ease,
        color 120ms ease;
}

.bar__tool:disabled {
    opacity: 0.35;
    cursor: default;
}

.bar__tool:hover:not(:disabled) {
    background-color: var(--jp-hover-bg, color-mix(in srgb, var(--ori-color-primary) 12%, transparent));
}

.bar__tool--active {
    background-color: var(--ori-color-primary);
    color: var(--ori-color-on-primary);
}

.bar__tool--active:hover:not(:disabled) {
    background-color: var(--ori-color-primary);
}

.bar__swatch {
    position: relative;
    display: grid;
    place-items: center;
}

.bar__swatch input[type='color'] {
    width: var(--jp-control-lg, 2.4rem);
    height: var(--jp-control-lg, 2.4rem);
    padding: 0;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 20%));
    border-radius: 50%;
    background: none;

    cursor: pointer;
}

/* Native color wells insist on their own swatch padding — trim it to the ring. */
.bar__swatch input[type='color']::-webkit-color-swatch-wrapper {
    padding: 2px;
}

.bar__swatch input[type='color']::-webkit-color-swatch {
    border: none;
    border-radius: 50%;
}

.bar__swatch input[type='color']:disabled {
    opacity: 0.35;
    cursor: default;
}

.bar__fill {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.bar__slider {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
    width: 9rem;
}

.bar__slider-value {
    min-width: 1.6rem;
    text-align: right;

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-variant-numeric: tabular-nums;
    color: var(--ori-color-on-surface);
    opacity: 0.75;
}

/* Phones: wrap so every control stays on-screen, tighten paddings, hide readout. */
@media (width <= 600px) {
    .bar {
        flex-wrap: wrap;
        justify-content: center;
        gap: var(--ori-size-gap_sm, 0.25rem);
        padding: 0.35rem 0.5rem;
    }

    .bar__tool {
        width: var(--jp-control-sm, 2.25rem);
        height: var(--jp-control-sm, 2.25rem);
    }

    .bar__swatch input[type='color'] {
        width: var(--jp-control-sm, 2.25rem);
        height: var(--jp-control-sm, 2.25rem);
    }

    .bar__slider {
        width: 5rem;
    }

    .bar__slider-value {
        display: none;
    }
}
</style>
