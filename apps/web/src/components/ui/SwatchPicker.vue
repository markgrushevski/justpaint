<script lang="ts">
/** One colour of a {@link SwatchPicker}. */
export interface SwatchOption {
    /** What choosing it emits; null is a choice too (the canvas's own paper). */
    value: string | null
    /** Accessible name, also the tooltip. */
    label: string
    /** The dot's paint, any CSS colour. */
    color: string
    /** The check mark's colour on this dot. */
    ink: string
}
</script>

<script lang="ts" setup>
/**
 * SwatchPicker — a single-select grid of colour dots (the accent, the canvas colour), and an
 * optional "custom" dot that opens oriui's colour picker. Each preset is an icon-mode
 * `OriButton` painted through the per-instance `--ori-color` escape hatch
 * (docs/DESIGN-SYSTEM.md §0), with radiogroup semantics and a tooltip naming it. `inkView`
 * shows the dots the way an inverted canvas shows a drawing, so a swatch looks like what it
 * paints.
 */
import { computed, ref } from 'vue'
import { OriButton, OriColorPicker, OriIcon, OriPopover, OriTooltip } from '@oriui/vue'
import { icons, inkOn } from '@core'

const props = withDefaults(
    defineProps<{
        modelValue: string | null
        options: SwatchOption[]
        /** Accessible name for the group. */
        label: string
        /** Dots per row. */
        perRow?: number
        inkView?: boolean
        /** Offer a picked colour after the presets: its name, and the colour it holds. */
        custom?: { label: string; color: string; active: boolean } | null
    }>(),
    { perRow: 5, inkView: false, custom: null }
)
const emit = defineEmits<{ 'update:modelValue': [value: string | null]; custom: [color: string] }>()

const group = ref<HTMLElement | null>(null)

const selectedIndex = computed(() =>
    props.custom?.active ? -1 : props.options.findIndex((o) => o.value === props.modelValue)
)
// The tab stop: the chosen dot, or the first while none of them is chosen.
const tabStop = computed(() => Math.max(0, selectedIndex.value))

function select(value: string | null): void {
    if (value !== props.modelValue || props.custom?.active) emit('update:modelValue', value)
}

/** Arrow keys move the selection (roving focus) — standard radiogroup semantics. */
function onKeydown(e: KeyboardEvent, index: number): void {
    const forward = e.key === 'ArrowRight' || e.key === 'ArrowDown'
    const back = e.key === 'ArrowLeft' || e.key === 'ArrowUp'
    if (!forward && !back) return
    e.preventDefault()
    const n = props.options.length
    const nextIndex = forward ? (index + 1) % n : (index - 1 + n) % n
    const next = props.options[nextIndex]
    if (!next) return
    select(next.value)
    group.value?.querySelectorAll<HTMLElement>('[role="radio"]')[nextIndex]?.focus()
}

// The picker edits a draft; only a settled colour (release, Enter, a preset) is emitted.
const draft = ref(props.custom?.color ?? '#ffffff')
function onOpen(): void {
    draft.value = props.custom?.color ?? draft.value
}
</script>

<template>
    <div class="swatches" :style="{ '--swatches-per-row': perRow }">
        <div ref="group" class="swatches__grid" role="radiogroup" :aria-label="label">
            <!-- The ring is ours, around the button: oriui keeps the dot itself. -->
            <span
                v-for="(opt, i) in options"
                :key="opt.label"
                class="swatches__ring"
                :class="{ 'swatches__ring--on': i === selectedIndex }"
            >
                <OriTooltip :content="opt.label" placement="bottom">
                    <OriButton
                        class="ori-button_icon swatches__dot"
                        :class="{ 'jp-ink-view': inkView }"
                        role="radio"
                        :aria-checked="i === selectedIndex"
                        :aria-label="opt.label"
                        :tabindex="i === tabStop ? 0 : -1"
                        variant="solid"
                        radius="full"
                        size="sm"
                        :style="{ '--ori-color': opt.color, '--ori-color-on': opt.ink }"
                        @click="select(opt.value)"
                        @keydown="onKeydown($event, i)"
                    >
                        <OriIcon v-if="i === selectedIndex" :icon="icons.mdiCheck" />
                    </OriButton>
                </OriTooltip>
            </span>
        </div>

        <span v-if="custom" class="swatches__ring" :class="{ 'swatches__ring--on': custom.active }">
            <OriPopover placement="bottom-end" :aria-label="custom.label">
                <template #trigger="{ props: trigger }">
                    <OriTooltip :content="custom.label" placement="bottom">
                        <OriButton
                            v-bind="trigger"
                            class="ori-button_icon"
                            :class="{ 'jp-ink-view': inkView && custom.active, swatches__dot: custom.active }"
                            :aria-label="custom.active ? `${custom.label}, ${custom.color}` : custom.label"
                            :aria-pressed="custom.active"
                            :variant="custom.active ? 'solid' : 'outline'"
                            color="surface"
                            radius="full"
                            size="sm"
                            :style="
                                custom.active
                                    ? { '--ori-color': custom.color, '--ori-color-on': inkOn(custom.color) }
                                    : {}
                            "
                            @click="onOpen"
                        >
                            <OriIcon :icon="custom.active ? icons.mdiCheck : icons.mdiPlus" />
                        </OriButton>
                    </OriTooltip>
                </template>
                <OriColorPicker v-model="draft" :label="custom.label" @change="(c: string) => emit('custom', c)" />
            </OriPopover>
        </span>
    </div>
</template>

<style scoped>
/* One grid for the presets and the custom dot: the radiogroup lays its dots out in it, so the
   custom dot takes the next cell, though it is no radio. */
.swatches {
    display: grid;
    grid-template-columns: repeat(var(--swatches-per-row), max-content);
    gap: var(--ori-size-gap_md, 0.5rem);
}

.swatches__grid {
    display: contents;
}

/* A hairline keeps a pale dot visible on the surface; the chosen one gets the accent. */
.swatches__ring {
    display: inline-flex;

    border-radius: 50%;
    box-shadow: 0 0 0 1px var(--jp-color-outline);
}

.swatches__ring--on {
    box-shadow:
        0 0 0 2px var(--ori-color-surface),
        0 0 0 4px var(--ori-color-primary);
}

/* A forced-colours mode would paint every dot the button colour, and a swatch is its colour,
   so the dots keep theirs. The mode drops the shadow rings; outlines in system colours stand
   in for them, the focused one outermost. */
@media (forced-colors: active) {
    .swatches__dot {
        forced-color-adjust: none;
    }

    .swatches__ring {
        outline: 1px solid CanvasText;
    }

    .swatches__ring--on {
        outline: 2px solid Highlight;
        outline-offset: 2px;
    }

    .swatches__ring:has(:focus-visible) {
        outline: 3px solid CanvasText;
        outline-offset: 4px;
    }
}
</style>
