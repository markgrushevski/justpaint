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
import { OriButton } from '@oriui/vue'
import { icons } from '@core'

withDefaults(defineProps<{ disabled?: boolean; loading?: boolean }>(), {
    disabled: false,
    loading: false
})

const emit = defineEmits<{ submit: [] }>()
</script>

<template>
    <OriButton
        class="submit"
        text="Submit"
        variant="fill"
        color="primary"
        radius="md"
        :icon="icons.mdiSwordCross"
        icon-position="left"
        :disabled="disabled"
        :loading="loading"
        @click="emit('submit')"
    />
</template>

<style scoped>
/* The button carries oriui's own fill-primary chrome; the shell's top-right
   region positions it. A soft shadow lifts it to match the .jp-float islands. */
.submit {
    box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}
</style>
