<script lang="ts" setup>
/**
 * The /draw empty-state card — an Excalidraw-inspired warm welcome shown
 * centered on a blank canvas (it replaces the old `.draw__hint` pill). A quick
 * launcher for the two things justpaint does: free-draw here, or a duel on
 * /play. Presentational — every action is an emit or a RouterLink; the host
 * (DrawView) owns the actual behavior and decides when to show/hide the card.
 *
 * The card sits on the shared `.jp-float` island language (surface + 1px
 * outline + soft shadow), same as the toolbar/zoom/layers chrome.
 */
import { OriButton } from '@oriui/vue'
import { RouterLink } from 'vue-router'
import { icons } from '@core'

const props = withDefaults(defineProps<{ signedIn?: boolean }>(), { signedIn: false })

const emit = defineEmits<{
    dismiss: []
    signIn: []
    shortcuts: []
}>()
</script>

<template>
    <div class="empty jp-float" role="group" aria-labelledby="empty-title">
        <h2 id="empty-title" class="empty__brand">justpaint</h2>
        <p class="empty__tagline">A tiny vector editor — and an AI-judged drawing duel.</p>

        <ul class="empty__actions">
            <li>
                <OriButton
                    class="empty__action"
                    text="Start drawing"
                    variant="text"
                    color="surface"
                    radius="md"
                    fluid
                    :icon="icons.mdiPencil"
                    icon-position="left"
                    @click="emit('dismiss')"
                />
            </li>
            <li>
                <!-- Rendered AS a RouterLink (renders an <a>): color=primary gives
                     the sanctioned AA role-as-text accent that oriui derives for
                     ori-color_primary text/plain variants. /play lands later. -->
                <OriButton
                    class="empty__action"
                    :as="RouterLink"
                    to="/play"
                    text="Play a duel"
                    variant="text"
                    color="primary"
                    radius="md"
                    fluid
                    :icon="icons.mdiSwordCross"
                    icon-position="left"
                />
            </li>
            <li v-if="!props.signedIn">
                <OriButton
                    class="empty__action"
                    text="Sign in"
                    variant="text"
                    color="surface"
                    radius="md"
                    fluid
                    :icon="icons.mdiLogin"
                    icon-position="left"
                    @click="emit('signIn')"
                />
            </li>
            <!-- Desktop only: no hardware keyboard on phones (mirrors how DrawView
                 hides .draw__chip-help <=600px). -->
            <li class="empty__row--desktop">
                <OriButton
                    class="empty__action"
                    text="Keyboard shortcuts"
                    variant="text"
                    color="surface"
                    radius="md"
                    fluid
                    :icon="icons.mdiKeyboard"
                    icon-position="left"
                    @click="emit('shortcuts')"
                />
            </li>
        </ul>
    </div>
</template>

<style scoped>
.empty {
    /* .jp-float supplies the border / radius / shadow; override its surface to the
       page background (white in light) so the brand wordmark clears the WCAG
       large-text 3:1 bar — #ff5500 is 2.85:1 on the surface but 3.21:1 on the
       background (matching how the card reads in the design mockup). */
    background-color: var(--ori-color-background);
    width: 300px;
    max-width: calc(100vw - 2rem);

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);

    padding: var(--ori-size-gap_lg, 0.75rem);
}

.empty__brand {
    margin: 0;

    font-weight: 700;
    font-size: 1.25rem;
    letter-spacing: -0.01em;
    color: var(--ori-color-primary);
}

.empty__tagline {
    margin: 0 0 var(--ori-size-gap_md, 0.5rem);

    font-size: var(--ori-font-size_sm, 0.875rem);
    line-height: 1.35;
    color: var(--ori-color-on-surface);
    /* 0.7 keeps the muted line past WCAG AA on the surface (matches the side
       menu's muted section hints). */
    opacity: 0.7;
}

.empty__actions {
    list-style: none;
    margin: 0;
    padding: 0;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);
}

/* Left-align the icon+label inside each full-width row (oriui centers by
   default). Unlayered, so it beats the layered .ori-button justify-content. */
.empty__action {
    justify-content: flex-start;
}

@media (width <= 600px) {
    .empty__row--desktop {
        display: none;
    }
}
</style>
