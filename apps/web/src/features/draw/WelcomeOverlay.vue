<script lang="ts" setup>
/**
 * The welcome over an empty /draw: the modes down the left, the wordmark in the middle
 * and hand-written hints at the chrome. Only the mode cards take the pointer, so a
 * stroke anywhere else lands on the canvas, and the view drops this on that stroke.
 */
import { RouterLink } from 'vue-router'
import { OriCard } from '@oriui/vue'
import { icons } from '@core'

const emit = defineEmits<{ start: [] }>()

const MODES = [
    { to: '/practice', title: 'Practice', subtitle: 'Draw a prompt, get an AI score', icon: icons.target },
    { to: '/play', title: 'Duel', subtitle: 'Draw against another player', icon: icons.mdiSwordCross },
    {
        to: '/gallery',
        title: 'My drawings',
        subtitle: 'Your saved sketches',
        icon: icons.mdiImageMultipleOutline
    }
]
</script>

<template>
    <section class="welcome" aria-label="Welcome">
        <!-- A few loose brush strokes as the welcome's ground; they go with it. -->
        <svg class="welcome__brush welcome__brush--left" viewBox="0 0 300 420" aria-hidden="true">
            <path d="M70 30 C 150 110, 30 220, 120 300 S 210 380, 170 400" />
        </svg>
        <svg class="welcome__brush welcome__brush--right" viewBox="0 0 360 120" aria-hidden="true">
            <path d="M14 96 C 90 30, 210 18, 344 56" />
        </svg>

        <nav class="welcome__modes" aria-label="Start">
            <!-- Draw is where the visitor already is: the card only clears the welcome. -->
            <button type="button" class="welcome__mode" @click="emit('start')">
                <OriCard
                    class="welcome__card"
                    variant="soft"
                    color="primary"
                    radius="lg"
                    :prepend-icon="icons.mdiPencil"
                    title="Draw"
                    subtitle="Quick sketches, visual notes"
                    data-ori-interactive
                />
            </button>
            <RouterLink v-for="m in MODES" :key="m.to" :to="m.to" class="welcome__mode">
                <OriCard
                    class="welcome__card"
                    variant="soft"
                    color="surface"
                    radius="lg"
                    :prepend-icon="m.icon"
                    :title="m.title"
                    :subtitle="m.subtitle"
                    data-ori-interactive
                />
            </RouterLink>
        </nav>

        <div class="welcome__center">
            <p class="welcome__brand">just<span class="welcome__brand-accent">paint</span></p>
            <p class="welcome__tagline">A simple canvas for your ideas.</p>
            <p class="welcome__motto" aria-hidden="true">
                Draw. Save. Keep it.
                <svg class="welcome__swoosh" viewBox="0 0 120 14" preserveAspectRatio="none">
                    <path d="M3 10 C 30 3, 70 2, 117 6" />
                </svg>
            </p>
            <p class="welcome__keys">Press <kbd>?</kbd> for keyboard shortcuts</p>
        </div>

        <!-- Pointers at the chrome; the controls they point at carry their own names. -->
        <div class="welcome__hint welcome__hint--top" aria-hidden="true">
            <svg class="welcome__arrow" viewBox="0 0 60 60">
                <path d="M8 54 C 14 30, 30 14, 52 8" />
                <path d="M40 5 L 53 8 L 46 19" />
            </svg>
            <span>Save, layers and AI live up here</span>
        </div>
        <div class="welcome__hint welcome__hint--tools" aria-hidden="true">
            <span>Pick a tool and start drawing</span>
            <svg class="welcome__arrow" viewBox="0 0 60 60">
                <path d="M16 6 C 34 14, 44 30, 40 52" />
                <path d="M31 43 L 40 54 L 48 42" />
            </svg>
        </div>
    </section>
</template>

<style scoped>
/* Covers the canvas but takes no pointer events: only the mode cards opt back in. */
.welcome {
    position: absolute;
    inset: 0;

    color: var(--ori-color-on-background);
    pointer-events: none;
}

.welcome__brush {
    position: absolute;

    fill: none;
    stroke-linecap: round;
}

/* The desk tone behind the mode column. */
.welcome__brush--left {
    top: 50%;
    left: -3rem;

    width: 20rem;
    height: 28rem;

    stroke: var(--jp-desk);
    stroke-width: 54;

    transform: translateY(-50%);
}

/* A faint orange sweep under the right half, clear of the zoom island. */
.welcome__brush--right {
    right: 8%;
    bottom: 8.5rem;

    width: 24rem;
    height: 8rem;

    stroke: var(--ori-color-primary);
    stroke-width: 18;
    opacity: 0.12;
}

.welcome__modes {
    position: absolute;
    top: 50%;
    left: var(--ori-size-gap_lg, 0.75rem);

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: 17rem;

    transform: translateY(-50%);
}

.welcome__mode {
    display: block;
    padding: 0;

    border: none;
    border-radius: var(--ori-size-radius_lg, 12px);
    background: none;

    color: inherit;
    font: inherit;
    text-align: left;
    text-decoration: none;

    cursor: pointer;
    pointer-events: auto;

    transition: transform 160ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.welcome__card {
    --ori-card-padding: var(--ori-size-gap_md, 0.5rem) var(--ori-size-gap_lg, 0.75rem);
}

@media (hover: hover) {
    .welcome__mode:hover {
        transform: translateX(4px);
    }
}

.welcome__mode:active {
    transform: scale(0.98);
}

.welcome__center {
    position: absolute;
    inset: 0;

    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--ori-size-gap_xs, 0.125rem);

    text-align: center;
}

.welcome__brand {
    margin: 0;

    font-weight: 800;
    font-size: 3.25rem;
    letter-spacing: -0.03em;
    line-height: 1.1;
}

.welcome__brand-accent {
    color: var(--ori-color-primary);
}

.welcome__tagline {
    margin: 0;

    font-size: var(--ori-font-size_lg, 1.125rem);
    opacity: 0.75;
}

.welcome__motto {
    position: relative;

    margin: var(--ori-size-gap_md, 0.5rem) 0 0;

    font-family: Caveat, cursive;
    font-size: 1.9rem;
    font-weight: 600;
}

.welcome__swoosh {
    position: absolute;
    right: -0.25rem;
    bottom: -0.35rem;

    width: 4.5rem;
    height: 0.6rem;

    fill: none;
    stroke: var(--ori-color-primary);
    stroke-width: 3;
    stroke-linecap: round;
}

.welcome__keys {
    margin: var(--ori-size-gap_lg, 0.75rem) 0 0;

    font-size: var(--ori-font-size_sm, 0.875rem);
    opacity: 0.7;
}

.welcome__keys kbd {
    padding: 0 0.35em;

    border: 1px solid var(--jp-color-outline);
    border-radius: var(--ori-size-radius_sm, 4px);

    font-family: inherit;
}

/* Hand-written pointers: large enough to count as large text at their opacity. */
.welcome__hint {
    position: absolute;

    display: flex;
    align-items: flex-end;
    gap: var(--ori-size-gap_xs, 0.125rem);

    font-family: Caveat, cursive;
    font-size: 1.6rem;
    font-weight: 600;
    line-height: 1;
    opacity: 0.8;
}

/* Hangs under the top-right island; the arrow's tip stops short of it. */
.welcome__hint--top {
    top: 4.25rem;
    right: 6rem;

    flex-direction: row-reverse;
    align-items: flex-end;
}

.welcome__hint--tools {
    bottom: 5.5rem;
    left: calc(50% - 16rem);
}

.welcome__arrow {
    flex: none;

    width: 3.25rem;
    height: 3.25rem;

    fill: none;
    stroke: currentcolor;
    stroke-width: 2.5;
    stroke-linecap: round;
    stroke-linejoin: round;
}

/* Between the phone breakpoint and a wide screen the column eats the left third, so
   the wordmark centers in what is left. */
@media (width <= 1100px) {
    .welcome__center {
        left: 18rem;
    }
}

@media (width <= 600px) {
    .welcome__brush,
    .welcome__keys,
    .welcome__hint--top {
        display: none;
    }

    .welcome__center {
        left: 0;
        justify-content: flex-start;

        padding-top: 22dvh;
    }

    .welcome__brand {
        font-size: 2.5rem;
    }

    /* The modes become a centered stack under the wordmark, clear of the hint and
       the zoom island below. */
    .welcome__modes {
        top: auto;
        bottom: 12rem;
        left: 50%;

        width: min(17rem, calc(100vw - 2rem));

        transform: translateX(-50%);
    }

    .welcome__mode:first-child {
        display: none;
    }

    .welcome__hint--tools {
        bottom: 8.25rem;
        left: 50%;

        font-size: 1.35rem;

        transform: translateX(-50%);
        white-space: nowrap;
    }
}

@media (height <= 560px) {
    .welcome__hint,
    .welcome__keys,
    .welcome__motto {
        display: none;
    }
}

@media (prefers-reduced-motion: reduce) {
    .welcome__mode {
        transition: none;
    }
}
</style>
