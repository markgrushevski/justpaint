<script lang="ts" setup>
/**
 * IconButton — the single toolbar/island icon action for the whole app. Wraps
 * `OriButton` so EVERY state comes from oriui props — `variant` / `pressed` /
 * `disabled`, the focus ring, the aria wiring, and the icon-mode square/circle
 * sizing (the public `ori-button_icon` class). It NEVER hand-rolls a `--active`
 * class, an `opacity` disable, or a `color-mix` of a brand role — that is the
 * whole point (docs/DESIGN-SYSTEM.md §2–§4). It renders the app icon set
 * (`ToolIcon`) so a cluster of these is always one glyph size.
 *
 * Defaults: `variant="text"` + `color="surface"` = a neutral ghost glyph. A
 * SELECTED/ON state passes `color="primary"` + `pressed` (the one place brand
 * colour enters a toggle); a PRIMARY action is a fill `OriButton`, not this.
 * `label` is the accessible name and the tooltip text (always shown — every
 * icon action gets a tooltip).
 */
import { OriButton, OriTooltip } from '@oriui/vue'
import type { Variant, ThemeColor, RadiusSize, AnchoredPlacement } from '@oriui/vue'
import ToolIcon from '../icons/ToolIcon.vue'
import type { IconName } from '../icons/ToolIcon.vue'

withDefaults(
    defineProps<{
        /** Glyph from the app icon set. */
        icon: IconName
        /** Accessible name + tooltip text (required — an icon needs a label). */
        label: string
        /**
         * Toggle state, for a button that IS a toggle (a panel opener). A real
         * boolean makes oriui render `aria-pressed` BOTH ways; leaving it
         * undefined means "not a toggle" and emits no ARIA state at all, which
         * is why it has no default. Replaces the `active` look plus a
         * hand-wired `aria-pressed` (oriui rc.18, JP-O-01).
         */
        pressed?: boolean
        disabled?: boolean
        /** Rest emphasis; default `text` (ghost). Selected states pass `tonal`/`fill`. */
        variant?: Variant
        /** Role colour; default `surface` (neutral). Selected/on passes `primary`. */
        color?: ThemeColor
        /** `md` = rounded square (default), `rounded` = circle. */
        radius?: RadiusSize
        /** Tooltip side. */
        placement?: AnchoredPlacement
    }>(),
    { disabled: false, variant: 'text', color: 'surface', radius: 'md', placement: 'top' }
)

const emit = defineEmits<{ click: [MouseEvent] }>()
</script>

<template>
    <OriTooltip :placement="placement" :content="label">
        <OriButton
            class="ori-button_icon"
            :variant="variant"
            :color="color"
            :radius="radius"
            :pressed="pressed"
            :disabled="disabled"
            :aria-label="label"
            @click="emit('click', $event)"
        >
            <ToolIcon :name="icon" />
        </OriButton>
    </OriTooltip>
</template>
