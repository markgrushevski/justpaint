<script lang="ts" setup>
import { OriButton } from '@oriui/vue'
import type { LayerView } from '@justpaint/editor'

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
}>()

// Display top layer first: the document orders layers bottom→top (layers[0] is
// the bottom), but a layers panel reads top-down. `index` in each row is the
// real document z-index; the up/down arrows move toward the top/bottom.
function rows() {
    return props.layers.map((layer, index) => ({ layer, index })).reverse()
}

// Commit opacity on release (`change`), not every `input` tick, so a whole
// slider drag collapses into a single undo step. The native thumb still tracks
// the drag visually; the document only updates on release.
function onOpacityChange(id: string, e: Event) {
    emit('setOpacity', id, Number((e.target as HTMLInputElement).value) / 100)
}

function onRename(id: string, e: Event) {
    emit('rename', id, (e.target as HTMLInputElement).value)
}

const top = () => props.layers.length - 1
</script>

<template>
    <aside class="layers">
        <header class="layers__head">
            <span class="layers__title">Layers</span>
            <OriButton size="sm" variant="outline" :disabled="!props.canAdd" @click="emit('add')">+ Add</OriButton>
        </header>

        <ul class="layers__list">
            <li
                v-for="{ layer, index } in rows()"
                :key="layer.id"
                class="layers__item"
                :class="{ 'layers__item--active': layer.id === props.activeLayerId }"
                @click="emit('select', layer.id)"
            >
                <div class="layers__row">
                    <input
                        class="layers__visible"
                        type="checkbox"
                        :checked="layer.visible"
                        :aria-label="`Toggle ${layer.name} visibility`"
                        @click.stop
                        @change="emit('toggleVisible', layer.id, ($event.target as HTMLInputElement).checked)"
                    />
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
                    <button
                        class="layers__btn"
                        title="Move up"
                        :disabled="index >= top()"
                        @click="emit('move', layer.id, index + 1)"
                    >
                        ↑
                    </button>
                    <button
                        class="layers__btn"
                        title="Move down"
                        :disabled="index <= 0"
                        @click="emit('move', layer.id, index - 1)"
                    >
                        ↓
                    </button>
                    <button
                        class="layers__btn layers__btn--danger"
                        title="Delete layer"
                        :disabled="props.layers.length <= 1"
                        @click="emit('remove', layer.id)"
                    >
                        ✕
                    </button>
                </div>
            </li>
        </ul>
    </aside>
</template>

<style scoped>
.layers {
    flex: none;
    width: 15rem;

    display: flex;
    flex-direction: column;
    gap: 0.5rem;

    padding: 0.75rem;

    border-left: 1px solid var(--ori-color-outline, rgb(0 0 0 / 10%));
    background-color: var(--ori-color-surface, #fafafa);
}

.layers__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
}

.layers__title {
    font-weight: 600;
    font-size: 0.9rem;
}

.layers__list {
    list-style: none;
    margin: 0;
    padding: 0;

    display: flex;
    flex-direction: column;
    gap: 0.375rem;

    overflow-y: auto;
}

.layers__item {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;

    padding: 0.5rem;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: 6px;

    cursor: pointer;
}

.layers__item--active {
    border-color: var(--ori-color-primary, #3b82f6);
    box-shadow: 0 0 0 1px var(--ori-color-primary, #3b82f6);
}

.layers__row {
    display: flex;
    align-items: center;
    gap: 0.375rem;
}

.layers__name {
    flex: 1 1 auto;
    min-width: 0;

    padding: 0.15rem 0.35rem;

    border: 1px solid transparent;
    border-radius: 4px;
    background: transparent;

    font-size: 0.85rem;
}

.layers__name:focus {
    border-color: var(--ori-color-outline, rgb(0 0 0 / 25%));
    background: var(--ori-color-background, #ffffff);
}

.layers__count {
    min-width: 1.5rem;
    text-align: right;

    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    color: var(--ori-color-on-surface-variant, #6b7280);
}

.layers__opacity {
    flex: 1 1 auto;
    min-width: 0;
}

.layers__btn {
    width: 1.5rem;
    height: 1.5rem;

    display: inline-flex;
    align-items: center;
    justify-content: center;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 20%));
    border-radius: 4px;
    background: var(--ori-color-background, #ffffff);

    cursor: pointer;
    font-size: 0.8rem;
    line-height: 1;
}

.layers__btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

.layers__btn--danger:not(:disabled):hover {
    border-color: var(--ori-color-danger, #c0392b);
    color: var(--ori-color-danger, #c0392b);
}
</style>
