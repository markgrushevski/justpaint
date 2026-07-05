<script lang="ts" setup>
import { OriButton, OriCheckbox } from '@oriui/vue'
import type { LayerView } from '@justpaint/editor'
import ToolIcon from './icons/ToolIcon.vue'

const props = defineProps<{
    layers: LayerView[]
    activeLayerId: string
    canAdd: boolean
}>()

const emit = defineEmits<{
    add: []
    select: [id: string]
    remove: [id: string]
    move: [id: string, toIndex: number]
    toggleVisible: [id: string, visible: boolean]
    setOpacity: [id: string, opacity: number]
    rename: [id: string, name: string]
    close: []
}>()

// Display top layer first: the document orders layers bottom→top (layers[0] is
// the bottom), but a layers panel reads top-down. `index` in each row is the
// real document z-index; the up/down arrows move toward the top/bottom.
function rows() {
    return props.layers.map((layer, index) => ({ layer, index })).reverse()
}

// Opacity commits on release (`change`), not every `input` tick, so a whole
// slider drag collapses into a single undo step. (This is why it stays a native
// range and not OriSlider, which would emit per-tick and flood history — unlike
// the toolbar width slider, which isn't recorded in history.)
function onOpacityChange(id: string, e: Event) {
    emit('setOpacity', id, Number((e.target as HTMLInputElement).value) / 100)
}

function onRename(id: string, e: Event) {
    emit('rename', id, (e.target as HTMLInputElement).value)
}

const top = () => props.layers.length - 1
</script>

<template>
    <aside class="layers jp-float" aria-label="Layers">
        <header class="layers__head">
            <span class="layers__title">Layers</span>
            <div class="layers__head-actions">
                <OriButton
                    text="+ Add"
                    size="sm"
                    variant="outline"
                    radius="md"
                    :disabled="!props.canAdd"
                    @click="emit('add')"
                />
                <button class="layers__close" type="button" aria-label="Close layers panel" @click="emit('close')">
                    <ToolIcon name="close" />
                </button>
            </div>
        </header>

        <ul class="layers__list">
            <li
                v-for="{ layer, index } in rows()"
                :key="layer.id"
                class="layers__item"
                :class="{ 'layers__item--active': layer.id === props.activeLayerId }"
                :aria-current="layer.id === props.activeLayerId ? 'true' : undefined"
                tabindex="0"
                @click="emit('select', layer.id)"
                @keydown.enter.self="emit('select', layer.id)"
                @keydown.space.self.prevent="emit('select', layer.id)"
            >
                <div class="layers__row">
                    <span class="layers__visible" @click.stop>
                        <OriCheckbox
                            :model-value="layer.visible"
                            :aria-label="`Toggle ${layer.name} visibility`"
                            @update:model-value="(v) => emit('toggleVisible', layer.id, v === true)"
                        />
                    </span>
                    <input
                        class="layers__name"
                        :value="layer.name"
                        :aria-label="`Rename ${layer.name}`"
                        @click.stop
                        @change="onRename(layer.id, $event)"
                    />
                    <span class="layers__count">{{ layer.strokeCount }}</span>
                </div>

                <div class="layers__row layers__row--controls" @click.stop>
                    <input
                        class="layers__opacity"
                        type="range"
                        min="0"
                        max="100"
                        step="1"
                        :value="Math.round(layer.opacity * 100)"
                        :aria-label="`${layer.name} opacity`"
                        @change="onOpacityChange(layer.id, $event)"
                    />
                    <OriButton
                        size="sm"
                        variant="outline"
                        radius="md"
                        :disabled="index >= top()"
                        aria-label="Move layer up"
                        @click="emit('move', layer.id, index + 1)"
                        >↑</OriButton
                    >
                    <OriButton
                        size="sm"
                        variant="outline"
                        radius="md"
                        :disabled="index <= 0"
                        aria-label="Move layer down"
                        @click="emit('move', layer.id, index - 1)"
                        >↓</OriButton
                    >
                    <OriButton
                        size="sm"
                        variant="outline"
                        color="danger"
                        radius="md"
                        :disabled="props.layers.length <= 1"
                        aria-label="Delete layer"
                        @click="emit('remove', layer.id)"
                        >✕</OriButton
                    >
                </div>
            </li>
        </ul>
    </aside>
</template>

<style scoped>
/* A floating island (the host positions it); .jp-float supplies the chrome. */
.layers {
    width: 100%;
    max-height: 100%;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);

    padding: var(--ori-size-gap_md, 0.5rem);
}

.layers__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
}

.layers__head-actions {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.layers__close {
    display: grid;
    place-items: center;

    width: 1.9rem;
    height: 1.9rem;

    border: none;
    border-radius: var(--ori-size-radius_md, 8px);
    background: transparent;
    color: var(--ori-color-on-surface);

    cursor: pointer;
}

.layers__close:hover {
    background-color: color-mix(in srgb, var(--ori-color-on-surface) 8%, transparent);
}

.layers__title {
    font-weight: 600;
    font-size: var(--ori-font-size_sm, 0.9rem);
    color: var(--ori-color-on-surface);
}

.layers__list {
    list-style: none;
    margin: 0;
    padding: 0;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);

    overflow-y: auto;
}

.layers__item {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);

    padding: var(--ori-size-gap_md, 0.5rem);

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_md, 8px);
    color: var(--ori-color-on-surface);

    cursor: pointer;
}

.layers__item--active {
    border-color: var(--ori-color-primary);
    box-shadow: 0 0 0 1px var(--ori-color-primary);
}

.layers__row {
    display: flex;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.layers__name {
    flex: 1 1 auto;
    min-width: 0;

    padding: 0.15rem 0.35rem;

    border: 1px solid transparent;
    border-radius: var(--ori-size-radius_sm, 4px);
    background: transparent;
    color: inherit;

    font-size: var(--ori-font-size_sm, 0.85rem);
}

.layers__name:focus {
    border-color: var(--ori-color-outline, rgb(0 0 0 / 25%));
    background: var(--ori-color-background);
}

.layers__count {
    min-width: 1.5rem;
    text-align: right;

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-variant-numeric: tabular-nums;
    opacity: 0.7;
}

.layers__opacity {
    flex: 1 1 auto;
    min-width: 0;
    accent-color: var(--ori-color-primary);
}

@media (width <= 600px) {
    .layers {
        max-height: 42dvh;
    }
}
</style>
