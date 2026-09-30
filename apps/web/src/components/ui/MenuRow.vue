<script lang="ts" setup>
/**
 * One row of a menu panel: an icon, a label, and a shortcut hint or a chevron into a
 * sub-panel. `to` makes it a link. oriui has no list row outside `OriMenu`, whose
 * role="menu" can't hold the panel's inline controls (docs/ISSUES-OUTER.md JP-O-13).
 */
import { RouterLink } from 'vue-router'
import { OriButton, OriIcon } from '@oriui/vue'
import { icons } from '@core'

withDefaults(
    defineProps<{
        /** An mdi path from `icons`. */
        icon: string
        label: string
        /** A shortcut shown on the right, e.g. "Ctrl+S". */
        hint?: string
        /** Opens a sub-panel: a chevron on the right. */
        chevron?: boolean
        /** A route: the row renders as a link. */
        to?: string
        disabled?: boolean
    }>(),
    { disabled: false }
)

const emit = defineEmits<{ click: [MouseEvent] }>()
</script>

<template>
    <OriButton
        class="menu-row"
        :as="to ? RouterLink : 'button'"
        :to="to"
        variant="text"
        color="surface"
        radius="md"
        fluid
        :disabled="disabled"
        @click="emit('click', $event)"
    >
        <OriIcon :icon="icon" class="menu-row__icon" />
        <span class="menu-row__label">{{ label }}</span>
        <kbd v-if="hint" class="menu-row__hint">{{ hint }}</kbd>
        <OriIcon v-if="chevron" :icon="icons.mdiChevronRight" class="menu-row__chevron" />
    </OriButton>
</template>

<style scoped>
/* oriui centers a button's content; a row reads left to right (the unlayered class
   wins over the layered default, as elsewhere in the app). */
.menu-row {
    justify-content: flex-start;
    gap: var(--ori-size-gap_md, 0.5rem);

    font-weight: 500;
}

.menu-row__icon {
    flex: none;
    opacity: 0.8;
}

.menu-row__label {
    flex: 1;
    text-align: left;
}

.menu-row__hint {
    font-family: inherit;
    font-size: var(--ori-font-size_xs, 0.75rem);
    opacity: 0.7;
}

.menu-row__chevron {
    flex: none;
    opacity: 0.6;
}
</style>
