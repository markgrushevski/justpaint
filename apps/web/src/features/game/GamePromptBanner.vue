<script lang="ts" setup>
/**
 * GamePromptBanner — the shared duel target, revealed centred above the
 * canvas: while the roster fills (`revealed = false`) it shows a redacted
 * "waiting for opponent" shimmer so neither player can pre-draw, and once
 * `drawing` starts (`revealed = true`) it reveals the prompt both duelists
 * must draw (docs/GAME.md §5). Presentational: PlayView owns the phase and
 * flips `revealed`; both states stay mounted and cross-fade via inline
 * opacity, not Vue `<Transition>`, which can stall on a leave and strand the
 * wrong text.
 */
import { OriSurface } from '@oriui/vue'

defineProps<{
    /** The prompt both players draw — shown only once revealed. */
    prompt: string
    /** false → redacted "waiting…"; true → the prompt text is shown. */
    revealed: boolean
    /**
     * No opponent exists for this banner, so the waiting layer must not either:
     * practice mounts the banner only once it has a prompt, and an unused
     * "waiting for opponent" layer would still be in the DOM for find-in-page
     * or a screen reader even while visually hidden.
     */
    solo?: boolean
}>()
</script>

<template>
    <OriSurface class="banner" role="status" aria-live="polite">
        <!-- Stacked in one grid cell; each layer's opacity binds straight to
             `revealed`, faded by the CSS transition on `.banner__layer`. -->
        <div
            v-if="!solo"
            class="banner__layer banner__layer--waiting"
            :style="{ opacity: revealed ? 0 : 1 }"
            :aria-hidden="revealed"
        >
            <span class="banner__dots" aria-hidden="true"><i></i><i></i><i></i></span>
            <span class="banner__waiting">Waiting for opponent…</span>
        </div>
        <div
            class="banner__layer banner__layer--prompt"
            :style="{ opacity: revealed ? 1 : 0 }"
            :aria-hidden="!revealed"
        >
            <span class="banner__label">Draw</span>
            <span class="banner__prompt">{{ prompt }}</span>
        </div>
    </OriSurface>
</template>

<style scoped>
.banner {
    /* Grid so both layers overlap in one cell — the pill sizes to the larger of
       the two states, so the reveal cross-fades with no width jump. */
    display: grid;

    max-width: min(90vw, 34rem);
    padding: 0.35rem 0.9rem;

    /* pointer-events:none so drawing passes through this centred readout. */
    pointer-events: none;
    user-select: none;
}

.banner__layer {
    grid-area: 1 / 1;

    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    min-height: 1.5rem;
}

.banner__label {
    flex: none;

    color: var(--ori-color-primary);

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-weight: 800;
    letter-spacing: 0.08em;
    text-transform: uppercase;
}

.banner__prompt {
    overflow: hidden;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_md, 1rem);
    font-weight: 700;
    letter-spacing: -0.01em;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.banner__waiting {
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.875rem);
    font-weight: 600;
    /* 0.7 keeps the muted line past WCAG AA on the surface. */
    opacity: 0.7;
}

/* Three shimmering dots standing in for the redacted prompt. */
.banner__dots {
    display: inline-flex;
    gap: 0.2rem;
}

.banner__dots i {
    width: 0.4rem;
    height: 0.4rem;

    border-radius: 50%;
    background-color: var(--ori-color-primary);

    animation: banner-blink 1.2s ease-in-out infinite;
}

.banner__dots i:nth-child(2) {
    animation-delay: 0.15s;
}

.banner__dots i:nth-child(3) {
    animation-delay: 0.3s;
}

@keyframes banner-blink {
    0%,
    100% {
        opacity: 0.25;
    }

    50% {
        opacity: 1;
    }
}

@media (prefers-reduced-motion: reduce) {
    .banner__dots i {
        animation: none;
        opacity: 0.7;
    }
}
</style>
