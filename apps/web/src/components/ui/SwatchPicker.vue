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
 * SwatchPicker — a single-select row of colour dots (the accent, the canvas background).
 * Each dot is an icon-mode `OriButton` painted through the per-instance `--ori-color`
 * escape hatch (docs/DESIGN-SYSTEM.md §0), with SegmentedControl's radiogroup semantics.
 * `inkView` shows the dots the way the dark theme shows a drawing, so a swatch looks like
 * what it paints.
 */
import { computed, ref } from 'vue'
import { OriButton, OriIcon } from '@oriui/vue'
import { icons } from '@core'

const props = defineProps<{
    modelValue: string | null
    options: SwatchOption[]
    /** Accessible name for the group. */
    label: string
    inkView?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string | null] }>()

const group = ref<HTMLElement | null>(null)

// The tab stop: the chosen dot, or the first while none of them is chosen.
const tabStop = computed(() =>
    Math.max(
        0,
        props.options.findIndex((o) => o.value === props.modelValue)
    )
)

function select(value: string | null): void {
    if (value !== props.modelValue) emit('update:modelValue', value)
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
</script>

<template>
    <div ref="group" class="swatches" role="radiogroup" :aria-label="label">
        <!-- The ring is ours, around the button: oriui keeps the dot itself. -->
        <span
            v-for="(opt, i) in options"
            :key="opt.label"
            class="swatches__ring"
            :class="{ 'swatches__ring--on': opt.value === modelValue }"
        >
            <OriButton
                class="ori-button_icon"
                :class="{ 'jp-ink-view': inkView }"
                role="radio"
                :aria-checked="opt.value === modelValue"
                :aria-label="opt.label"
                :title="opt.label"
                :tabindex="i === tabStop ? 0 : -1"
                variant="solid"
                radius="full"
                size="sm"
                :style="{ '--ori-color': opt.color, '--ori-color-on': opt.ink }"
                @click="select(opt.value)"
                @keydown="onKeydown($event, i)"
            >
                <OriIcon v-if="opt.value === modelValue" :icon="icons.mdiCheck" />
            </OriButton>
        </span>
    </div>
</template>

<style scoped>
.swatches {
    display: flex;
    flex-wrap: wrap;
    gap: var(--ori-size-gap_md, 0.5rem);
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
</style>
