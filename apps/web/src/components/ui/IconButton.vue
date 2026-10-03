<script lang="ts" setup>
/**
 * IconButton — the single toolbar/island icon action for the whole app: wraps
 * `OriButton` so every state comes from oriui props (`variant`/`pressed`/
 * `disabled`, focus ring, aria wiring, icon-mode sizing) rather than a
 * hand-rolled `--active` class, `opacity` disable, or brand `color-mix`
 * (docs/DESIGN-SYSTEM.md §2). Defaults to a neutral ghost glyph
 * (`variant="text"` + `color="surface"`); a selected/on state passes
 * `color="primary"` + `pressed`, and a primary action is a fill `OriButton`,
 * not this. `label` doubles as the accessible name and the tooltip text.
 */
import { useId } from 'vue'
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
         * Toggle state for a button that is a toggle (a panel opener). Leaving
         * it undefined means "not a toggle" and emits no `aria-pressed` at
         * all, which is why it has no default.
         */
        pressed?: boolean
        disabled?: boolean
        /** Rest emphasis; default `text` (ghost). Selected states pass `soft`/`solid`. */
        variant?: Variant
        /** Role colour; default `surface` (neutral). Selected/on passes `primary`. */
        color?: ThemeColor
        /** `md` = rounded square (default), `full` = circle. */
        radius?: RadiusSize
        placement?: AnchoredPlacement
    }>(),
    { disabled: false, variant: 'text', color: 'surface', radius: 'md', placement: 'top' }
)

const emit = defineEmits<{ click: [MouseEvent] }>()

// Every oriui tooltip shares one anchor name, and a bubble can end up on another trigger
// (docs/ISSUES-OUTER.md); a name of its own pairs it with this one.
const anchor = { '--ori-anchor': `--jp-tip-${useId()}` }
</script>

<template>
    <OriTooltip :placement="placement" :content="label" :style="anchor">
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
