<script lang="ts" setup>
/**
 * SubmitButton — the one accent action of a live round: lock in your drawing to
 * be rendered + judged. It occupies the top-right slot that /draw gives to Save
 * (DECISIONS 2026-07-04: one design, game chrome on top), so a duel reads as the
 * same shell wearing game clothes.
 *
 * Presentational: emits `submit`; PlayView owns disabled/loading and the actual
 * (later: server) submit. Kept as the sole filled-primary control on the page.
 */
import { computed } from 'vue'
import { OriButton } from '@oriui/vue'
import { icons } from '@core'

const props = withDefaults(defineProps<{ disabled?: boolean; loading?: boolean; solo?: boolean }>(), {
    disabled: false,
    loading: false,
    /**
     * `solo` swaps the glyph, the third of the shared game components to carry
     * this flag (GamePromptBanner, JudgingOverlay). The action is genuinely the
     * same in both modes — lock the drawing in to be rendered and scored — so the
     * component stays one; only the crossed swords are a claim practice cannot
     * make, since there is nobody to cross them with. The bullseye is practice's
     * own glyph everywhere else it is named.
     */
    solo: false
})

const icon = computed(() => (props.solo ? icons.target : icons.mdiSwordCross))

const emit = defineEmits<{ submit: [] }>()
</script>

<template>
    <OriButton
        class="submit"
        text="Submit"
        variant="fill"
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
    box-shadow: var(--ori-shadow-lg);
}
</style>
