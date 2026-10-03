<script lang="ts" setup>
/**
 * The welcome over an empty /draw: the modes down the left, the wordmark in the middle
 * and pointers at the chrome. Only the mode cards take the pointer, so a
 * stroke anywhere else lands on the canvas, and the view drops this on that stroke.
 */
import { RouterLink } from 'vue-router'
import { OriCard } from '@oriui/vue'
import { icons } from '@core'

const emit = defineEmits<{ start: [] }>()

const MODES = [
    { to: '/practice', title: 'Practice', subtitle: 'Draw a prompt, get an AI score', icon: icons.target },
    { to: '/play', title: 'Duel', subtitle: 'Draw against another player', icon: icons.mdiSwordCross }
]
</script>

<template>
    <section class="welcome" aria-label="Welcome">
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
            <!-- The modes above, a place below. -->
            <hr class="welcome__rule" />
            <RouterLink to="/gallery" class="welcome__mode">
                <OriCard
                    class="welcome__card"
                    variant="soft"
                    color="surface"
                    radius="lg"
                    :prepend-icon="icons.mdiImageMultipleOutline"
                    title="My drawings"
                    subtitle="Your saved sketches"
                    data-ori-interactive
                />
            </RouterLink>
        </nav>

        <div class="welcome__center">
            <p class="welcome__brand">just<span class="welcome__brand-accent">paint</span></p>
            <p class="welcome__tagline">A sketchbook with an AI judge.</p>
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

.welcome__modes {
    position: absolute;
    top: 50%;
    left: var(--ori-size-gap_lg, 0.75rem);

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);

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

.welcome__rule {
    width: 100%;
    margin: var(--ori-size-gap_sm, 0.25rem) 0;

    border: none;
    border-top: 1px solid var(--jp-color-outline);
    opacity: 0.35;
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
    opacity: var(--jp-dim, 0.75);
}

.welcome__keys {
    margin: var(--ori-size-gap_lg, 0.75rem) 0 0;

    font-size: var(--ori-font-size_sm, 0.875rem);
    opacity: var(--jp-dim, 0.7);
}

.welcome__keys kbd {
    padding: 0 0.35em;

    border: 1px solid var(--jp-color-outline);
    border-radius: var(--ori-size-radius_sm, 4px);

    font-family: inherit;
}

/* Pointers at the chrome; 0.75 keeps the text well past AA on the paper. */
.welcome__hint {
    position: absolute;

    display: flex;
    align-items: flex-end;
    gap: var(--ori-size-gap_xs, 0.125rem);

    font-size: 1.125rem;
    font-weight: 700;
    line-height: 1.2;
    opacity: var(--jp-dim, 0.75);
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

        font-size: 1rem;

        transform: translateX(-50%);
        white-space: nowrap;
    }
}

@media (height <= 560px) {
    .welcome__hint,
    .welcome__keys {
        display: none;
    }
}

@media (prefers-reduced-motion: reduce) {
    .welcome__mode {
        transition: none;
    }
}
</style>
