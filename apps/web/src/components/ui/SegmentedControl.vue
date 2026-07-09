<script lang="ts">
import type { IconName } from '../icons/ToolIcon.vue'

/** One segment of a {@link SegmentedControl}. */
export interface SegmentOption {
    value: string
    label: string
    icon?: IconName
}
</script>

<script lang="ts" setup>
/**
 * SegmentedControl — a single-select segmented button group (a compact settings
 * picker like the theme Light/Dark/Auto). It COMPOSES oriui: `.ori-join` collapses
 * the segments' adjacent borders/radii into one unit, and each segment is an
 * `OriButton` (selected = `fill`, others = `outline` — the colour comes from
 * props, never a hand-rolled `--active` class or a brand `color-mix`, §1). What it
 * adds over a bare `OriJoin` is the single-select model + radiogroup a11y
 * (`role="radiogroup"` / `role="radio"` segments, `aria-checked`, roving tabindex,
 * Arrow-key selection). oriui also ships `OriRadioGroup` (native radio-circle
 * single-select), but that's the wrong VISUAL here — we want a segmented button
 * look (docs/DESIGN-SYSTEM.md §4).
 */
import { ref } from 'vue'
import { OriButton } from '@oriui/vue'
import type { ThemeColor } from '@oriui/vue'
import ToolIcon from '../icons/ToolIcon.vue'

const props = withDefaults(
    defineProps<{
        modelValue: string
        options: SegmentOption[]
        /** Accessible name for the group (becomes the radiogroup's aria-label). */
        label: string
        /** Accent colour of the selected segment. */
        color?: ThemeColor
    }>(),
    { color: 'primary' }
)
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const group = ref<HTMLElement | null>(null)

function select(value: string): void {
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
    <div ref="group" class="seg ori-join" role="radiogroup" :aria-label="label">
        <OriButton
            v-for="(opt, i) in options"
            :key="opt.value"
            class="seg__item"
            role="radio"
            :aria-checked="opt.value === modelValue"
            :tabindex="opt.value === modelValue ? 0 : -1"
            :variant="opt.value === modelValue ? 'fill' : 'outline'"
            :color="opt.value === modelValue ? color : 'surface'"
            radius="md"
            size="sm"
            fluid
            @click="select(opt.value)"
            @keydown="onKeydown($event, i)"
        >
            <ToolIcon v-if="opt.icon" :name="opt.icon" />
            <span class="seg__label">{{ opt.label }}</span>
        </OriButton>
    </div>
</template>

<style scoped>
/* `.ori-join` (oriui) collapses the segments' adjacent borders + radii into one
   segmented unit — the selected one fills, the rest are outlined. We only stretch
   it to full width and make the segments share the space equally. */
.seg {
    display: flex;
    width: 100%;
}

.seg__item {
    flex: 1;
}

.seg__label {
    font-weight: 600;
}
</style>
