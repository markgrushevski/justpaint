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
 * picker like the theme Light/Dark/Auto). oriui ships no segmented / radio-group
 * primitive (OriTabs is a view-switching tablist with label-only items and
 * tabpanel semantics; OriSwitch is binary), so this is a justpaint UI primitive
 * filling that gap (docs/DESIGN-SYSTEM.md §4, §6). Built ON oriui: each segment
 * is an `OriButton` whose SELECTED state is the `fill` variant and unselected is
 * `text` — the colour comes from oriui props, never a hand-rolled `--active`
 * class or a brand `color-mix` (§1). Proper radiogroup a11y: `role="radiogroup"`
 * with `role="radio"` segments, `aria-checked`, roving tabindex, and Arrow-key
 * selection.
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
    <div ref="group" class="seg" role="radiogroup" :aria-label="label">
        <OriButton
            v-for="(opt, i) in options"
            :key="opt.value"
            class="seg__item"
            role="radio"
            :aria-checked="opt.value === modelValue"
            :tabindex="opt.value === modelValue ? 0 : -1"
            :variant="opt.value === modelValue ? 'fill' : 'text'"
            :color="opt.value === modelValue ? color : 'surface'"
            radius="sm"
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
/* A subtle bordered track holding the segments; the selected one fills (oriui
   `fill` variant), so the track just groups them. Full-width by default. */
.seg {
    display: flex;
    gap: 2px;

    padding: 2px;

    border: 1px solid var(--ori-color-outline, rgb(0 0 0 / 12%));
    border-radius: var(--ori-size-radius_md, 8px);
}

.seg__item {
    flex: 1;
}

.seg__label {
    font-weight: 600;
}
</style>
