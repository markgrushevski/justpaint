<script lang="ts" setup>
/**
 * The /draw empty-state card — a launcher shown centered on a blank canvas:
 * free-draw here, a duel on /play, or a solo run on /practice. Presentational:
 * every action is an emit or a RouterLink, and the host (DrawView) decides
 * when to show or hide it.
 */
import { OriButton, OriSurface } from '@oriui/vue'
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
    <OriSurface class="empty" role="group" aria-labelledby="empty-title">
        <h2 id="empty-title" class="empty__brand">justpaint</h2>
        <p class="empty__tagline">A tiny vector editor — and an AI-judged drawing duel.</p>

        <ul class="empty__actions">
            <li>
                <OriButton
                    class="empty__action"
                    label="Start drawing"
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
                <!-- RouterLink renders an <a>; color="primary" is the AA-safe
                     text-role accent oriui derives for text/quiet variants. -->
                <OriButton
                    class="empty__action"
                    :as="RouterLink"
                    to="/play"
                    label="Play a duel"
                    variant="text"
                    color="primary"
                    radius="md"
                    fluid
                    :icon="icons.mdiSwordCross"
                    icon-position="left"
                />
            </li>
            <li>
                <!-- Not gated on `signedIn` (unlike Leaderboard below): /practice
                     raises the shared sign-in modal itself and drops into a
                     prompt, so an anonymous visitor lands somewhere real
                     instead of a 401. -->
                <OriButton
                    class="empty__action"
                    :as="RouterLink"
                    to="/practice"
                    label="Practice solo"
                    variant="text"
                    color="primary"
                    radius="md"
                    fluid
                    :icon="icons.target"
                    icon-position="left"
                />
            </li>
            <li v-if="props.signedIn">
                <!-- Signed-in only: /leaderboard requires a session, so an
                     anonymous visitor would hit a 401 (mirrors the "Sign in"
                     row / SideMenu). -->
                <OriButton
                    class="empty__action"
                    :as="RouterLink"
                    to="/leaderboard"
                    label="Leaderboard"
                    variant="text"
                    color="surface"
                    radius="md"
                    fluid
                    :icon="icons.podium"
                    icon-position="left"
                />
            </li>
            <li v-if="!props.signedIn">
                <OriButton
                    class="empty__action"
                    label="Sign in"
                    variant="text"
                    color="surface"
                    radius="md"
                    fluid
                    :icon="icons.mdiLogin"
                    icon-position="left"
                    @click="emit('signIn')"
                />
            </li>
            <!-- Desktop only: no hardware keyboard on phones (mirrors DrawView
                 hiding .draw__chip-help <=600px). -->
            <li class="empty__row--desktop">
                <OriButton
                    class="empty__action"
                    label="Keyboard shortcuts"
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
    </OriSurface>
</template>

<style scoped>
.empty {
    /* Override OriSurface's surface color to the page background so the brand
       wordmark clears the WCAG large-text 3:1 bar: #ff5500 is 2.85:1 on the
       surface but 3.21:1 on the background. */
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
    /* 0.7 keeps the muted line past WCAG AA on the surface. */
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

/* Left-aligns the icon+label; oriui centers by default. Unlayered, so it
   beats the layered .ori-button justify-content. */
.empty__action {
    justify-content: flex-start;
}

@media (width <= 600px) {
    .empty__row--desktop {
        display: none;
    }
}
</style>
