<script lang="ts" setup>
import { OriButton } from '@oriui/vue'
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
function onWidth(e: Event) {
    const value = Number((e.target as HTMLInputElement).value)
    if (Number.isFinite(value) && value > 0) emit('setWidth', value)
}
function onToggleFill(e: Event) {
    emit('toggleFill', (e.target as HTMLInputElement).checked)
}
function onFill(e: Event) {
    emit('setFill', (e.target as HTMLInputElement).value)
}

// `props` is referenced through the template; alias for clarity in script.
void props
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
            <label class="toolbar__field">
                <span>Color</span>
                <input type="color" :value="props.color" @input="onColor" />
            </label>

            <label class="toolbar__field">
                <span>Width</span>
                <input type="range" min="1" max="64" step="1" :value="props.strokeWidth" @input="onWidth" />
                <span class="toolbar__readout">{{ props.strokeWidth }}</span>
            </label>

            <label class="toolbar__field">
                <input type="checkbox" :checked="props.fillEnabled" @change="onToggleFill" />
                <span>Fill</span>
                <input type="color" :value="props.fill" :disabled="!props.fillEnabled" @input="onFill" />
            </label>
        </div>

        <div class="toolbar__group toolbar__group--actions">
            <OriButton variant="outline" size="sm" :disabled="!props.canUndo" @click="emit('undo')">Undo</OriButton>
            <OriButton variant="outline" size="sm" :disabled="!props.canRedo" @click="emit('redo')">Redo</OriButton>
            <OriButton variant="outline" size="sm" @click="emit('clear')">New</OriButton>
            <OriButton variant="outline" size="sm" @click="emit('exportPng')">Export PNG</OriButton>
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
    gap: 1rem;

    padding: 0.5rem 0.75rem;

    background-color: var(--ori-color-surface);
    border-bottom: 1px solid var(--ori-color-outline, rgb(0 0 0 / 10%));
}

.toolbar__group {
    display: flex;
    align-items: center;
    gap: 0.375rem;
}

.toolbar__group--actions {
    margin-left: auto;
}

.toolbar__field {
    display: flex;
    align-items: center;
    gap: 0.375rem;

    font-size: 0.875rem;
}

.toolbar__field input[type='color'] {
    width: 2rem;
    height: 1.75rem;
    padding: 0;

    cursor: pointer;
}

.toolbar__readout {
    min-width: 1.5rem;
    text-align: right;
    font-variant-numeric: tabular-nums;
}

.toolbar__message {
    flex-basis: 100%;
    margin: 0;

    font-size: 0.875rem;
    color: var(--ori-color-primary);
}
</style>
