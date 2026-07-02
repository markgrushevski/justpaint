<script lang="ts" setup>
import { OriButton, OriCheckbox, OriSlider } from '@oriui/vue'
import { TOOLS } from '@justpaint/editor'
import type { ToolId } from '@justpaint/editor'

const toolIds = Object.keys(TOOLS) as ToolId[]

const TOOL_LABELS: Record<ToolId, string> = {
    pen: 'Pen',
    eraser: 'Eraser',
    line: 'Line',
    rect: 'Rect',
    ellipse: 'Ellipse',
    triangle: 'Triangle'
}

const props = defineProps<{
    activeTool: ToolId
    color: string
    strokeWidth: number
    fillEnabled: boolean
    fill: string
    busy: boolean
    canUndo: boolean
    canRedo: boolean
    message: string | null
}>()

const emit = defineEmits<{
    pickTool: [id: ToolId]
    setColor: [hex: string]
    setWidth: [width: number]
    toggleFill: [enabled: boolean]
    setFill: [hex: string]
    undo: []
    redo: []
    clear: []
    exportPng: []
    save: []
    load: []
}>()

function onColor(e: Event) {
    emit('setColor', (e.target as HTMLInputElement).value)
}
function onFill(e: Event) {
    emit('setFill', (e.target as HTMLInputElement).value)
}
</script>

<template>
    <div class="toolbar">
        <div class="toolbar__group">
            <OriButton
                v-for="id in toolIds"
                :key="id"
                :variant="props.activeTool === id ? 'fill' : 'outline'"
                :active="props.activeTool === id"
                :aria-pressed="props.activeTool === id"
                size="sm"
                @click="emit('pickTool', id)"
            >
                {{ TOOL_LABELS[id] }}
            </OriButton>
        </div>

        <div class="toolbar__group">
            <label class="toolbar__swatch" title="Stroke color">
                <input type="color" :value="props.color" aria-label="Stroke color" @input="onColor" />
            </label>

            <div class="toolbar__slider">
                <OriSlider
                    :model-value="props.strokeWidth"
                    :min="1"
                    :max="64"
                    :step="1"
                    label="Width"
                    :show-value="true"
                    @update:model-value="(v: number) => emit('setWidth', v)"
                />
            </div>

            <OriCheckbox
                :model-value="props.fillEnabled"
                label="Fill"
                @update:model-value="(v) => emit('toggleFill', v === true)"
            />
            <label class="toolbar__swatch" title="Fill color">
                <input
                    type="color"
                    :value="props.fill"
                    :disabled="!props.fillEnabled"
                    aria-label="Fill color"
                    @input="onFill"
                />
            </label>
        </div>

        <div class="toolbar__group toolbar__group--actions">
            <OriButton variant="outline" size="sm" :disabled="!props.canUndo" @click="emit('undo')">Undo</OriButton>
            <OriButton variant="outline" size="sm" :disabled="!props.canRedo" @click="emit('redo')">Redo</OriButton>
            <OriButton variant="outline" size="sm" @click="emit('clear')">New</OriButton>
            <OriButton variant="outline" size="sm" @click="emit('exportPng')">Export</OriButton>
            <OriButton variant="tonal" size="sm" :loading="props.busy" @click="emit('save')">Save</OriButton>
            <OriButton variant="tonal" size="sm" :loading="props.busy" @click="emit('load')">Load</OriButton>
        </div>

        <p v-if="props.message" class="toolbar__message" role="status">{{ props.message }}</p>
    </div>
</template>

<style scoped>
.toolbar {
    flex: none;

    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--ori-size-gap_lg, 1rem);

    padding: var(--ori-size-gap_md, 0.5rem) var(--ori-size-gap_lg, 0.75rem);

    background-color: var(--ori-color-surface);
    border-bottom: 1px solid var(--ori-color-outline, rgb(0 0 0 / 10%));
}

.toolbar__group {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.375rem);
    flex-wrap: wrap;
}

.toolbar__group--actions {
    margin-left: auto;
}

.toolbar__slider {
    width: 9rem;
}

.toolbar__swatch input[type='color'] {
    width: 2rem;
    height: 1.75rem;
    padding: 0;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 20%));
    border-radius: var(--ori-size-radius_sm, 4px);
    background: none;

    cursor: pointer;
}

.toolbar__swatch input[type='color']:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

.toolbar__message {
    flex-basis: 100%;
    margin: 0;

    font-size: var(--ori-font-size_sm, 0.875rem);
    color: var(--ori-color-on-surface);
}

@media (width <= 600px) {
    .toolbar__group--actions {
        margin-left: 0;
    }
}
</style>
