<script lang="ts" setup>
/**
 * SubmitButton — the one accent action of a live round: lock the drawing in
 * to be rendered and judged, in the top-right slot that /draw gives to Save,
 * so a duel reads as the same shell wearing game clothes. Presentational:
 * emits `submit`; the host view owns disabled/loading and the actual submit.
 * The sole filled-primary control on the page.
 */
import { computed } from 'vue'
import { OriButton } from '@oriui/vue'
import { icons } from '@core'

const props = withDefaults(defineProps<{ disabled?: boolean; loading?: boolean; solo?: boolean }>(), {
    disabled: false,
    loading: false,
    /**
     * `solo` swaps the glyph only: the action is the same in both modes (lock
     * the drawing in to be scored), but crossed swords are a claim practice
     * can't make — there's nobody to cross them with. The bullseye is
     * practice's own glyph everywhere else it's named.
     */
    solo: false
})

const icon = computed(() => (props.solo ? icons.target : icons.mdiSwordCross))

const emit = defineEmits<{ submit: [] }>()
</script>

<template>
    <OriButton
        class="submit"
        label="Submit"
        variant="solid"
        color="primary"
        radius="md"
        :icon="icon"
        icon-position="left"
        :disabled="disabled"
        :loading="loading"
        @click="emit('submit')"
    />
</template>

<style scoped>
/* The button carries oriui's own fill-primary chrome; the shell's top-right
   region positions it. A soft shadow lifts it to match the OriSurface islands. */
.submit {
    box-shadow: var(--ori-shadow-md);
}
</style>
