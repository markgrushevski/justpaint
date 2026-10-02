<script lang="ts" setup>
/**
 * GamePromptBanner — the round's prompt, "Draw {prompt}". On an empty canvas
 * (`large`) it is a tilted card dealt in from above; from the first stroke it
 * is a one-line pill. The duel's waiting state (`revealed = false`) is always a
 * pill, so neither player can pre-draw (docs/GAME.md §5). Presentational:
 * PlayView owns the phase and flips `revealed`. Both layers stay mounted in one
 * grid cell and switch on inline opacity, not Vue `<Transition>`, which can
 * stall on a leave and strand the wrong text.
 */
import { computed, ref, watch } from 'vue'
import { OriSurface } from '@oriui/vue'

const props = defineProps<{
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
    /** The opening look on an empty canvas: the prompt dealt as a card. */
    large?: boolean
}>()

/**
 * True once the card has been dealt. Only a card that is put away animates
 * into the pill: a banner that mounts as a pill never was a card.
 */
const dealt = ref(false)
watch(
    () => props.large,
    (large) => {
        if (large) dealt.value = true
    },
    { immediate: true }
)
const pocketing = computed(() => dealt.value && !props.large)
</script>

<template>
    <OriSurface
        class="banner"
        :class="{ 'banner--large': large, 'banner--pocket': pocketing, 'banner--duel': !solo }"
        role="status"
        aria-live="polite"
    >
        <!-- Stacked in one grid cell; each layer's opacity binds straight to `revealed`. -->
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
            <span class="banner__prompt">Draw {{ prompt }}</span>
        </div>
    </OriSurface>
</template>

<style scoped>
.banner {
    --banner-top: 0;

    /* Grid so both layers overlap in one cell — the pill sizes to the larger of
       the two states. */
    display: grid;

    max-width: min(90vw, 34rem);
    margin-top: var(--banner-top);
    padding: 0.35rem 0.9rem;

    /* The top edge stays put while the card turns and shrinks. */
    transform-origin: 50% 0;

    /* pointer-events:none so drawing passes through this centred readout. */
    pointer-events: none;
    user-select: none;
}

/* The top margin keeps the tilted corner that swings upward on the page. */
.banner--large {
    --banner-top: 0.4rem;

    max-width: min(90vw, 22rem);
    padding: 1.1rem 1.4rem 1.25rem;

    transform: rotate(-2deg);

    animation: banner-deal 420ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

/* A layout change can't be tweened between a wrapped card and a nowrap pill, so
   the pill itself starts from the card's tilt and a larger size and settles. */
.banner--pocket {
    animation: banner-pocket 220ms ease-out;
}

.banner__layer {
    grid-area: 1 / 1;

    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    min-height: 1.5rem;
}

.banner__prompt {
    overflow: hidden;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_md, 1rem);
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.banner--large .banner__layer--prompt {
    justify-content: flex-start;
}

/* Display type that wraps rather than truncates: the whole prompt is the point
   of the card. 24px on a phone, 36px from tablet up. */
.banner--large .banner__prompt {
    overflow: visible;

    font-size: clamp(1.5rem, 0.5rem + 4vw, 2.25rem);
    font-weight: 900;
    letter-spacing: -0.02em;
    line-height: 1.1;
    text-align: left;
    text-wrap: balance;
    white-space: normal;
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

/* The implicit end frame is the card's own tilt, so the overshoot settles on it. */
@keyframes banner-deal {
    from {
        transform: translateY(-2.5rem) rotate(-9deg);
    }
}

@keyframes banner-pocket {
    from {
        transform: rotate(-2deg) scale(1.25);
    }
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

/* On a phone the duel's opponent chip sits under the mode switcher at the left,
   and the card and the pill are wide enough to cover it. */
@media (width <= 600px) {
    .banner--duel {
        --banner-top: 3.25rem;
    }
}

@media (prefers-reduced-motion: reduce) {
    .banner--large,
    .banner--pocket {
        animation: none;
    }

    .banner__dots i {
        animation: none;
        opacity: 0.7;
    }
}
</style>
