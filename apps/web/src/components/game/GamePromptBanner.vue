<script lang="ts" setup>
/**
 * GamePromptBanner — the shared duel target, revealed centred above the canvas.
 * While the roster is still filling (`revealed = false`) it shows a redacted
 * "waiting for opponent" shimmer so neither player can pre-draw; once the match
 * enters the drawing phase (`revealed = true`) it reveals the single prompt both
 * duelists must draw (GAME.md §5 — same prompt, revealed at the same moment).
 *
 * Presentational: PlayView owns the phase and flips `revealed`. The pill is
 * `pointer-events: none` so a stroke can still start on the canvas beneath it.
 *
 * Both states are always mounted, stacked in one grid cell, and cross-faded by
 * the `banner--revealed` class (pure CSS) — no Vue <Transition mode="out-in">,
 * which can stall waiting on a leave and strand the wrong text on screen.
 */
import { OriSurface } from '@oriui/vue'

defineProps<{
    /** The prompt both players draw — shown only once revealed. */
    prompt: string
    /** false → redacted "waiting…"; true → the prompt text is shown. */
    revealed: boolean
}>()
</script>

<template>
    <OriSurface class="banner" role="status" aria-live="polite">
        <!-- Both states stay mounted, stacked in one grid cell, cross-faded by an
             inline opacity bound straight to `revealed` — no descendant-combinator
             cascade, no Vue <Transition> to stall; the fade is the CSS transition
             on `.banner__layer`. -->
        <div
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
    /* 0.7 keeps the muted line past WCAG AA on the surface (matches the shell). */
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
