<script lang="ts">
/**
 * Score bands, highest first. A number alone isn't a verdict, so the screen
 * leads with the band and lets the score card back it up. Copy, not contract:
 * nothing downstream reads these thresholds.
 */
const BANDS: { min: number; headline: string }[] = [
    { min: 0.85, headline: 'Nailed it' },
    { min: 0.65, headline: 'Close' },
    { min: 0.45, headline: 'Getting there' },
    { min: 0.25, headline: 'Some of it landed' },
    { min: 0, headline: 'Not this time' }
]
</script>

<script lang="ts" setup>
/**
 * PracticeResult — the payoff screen for a single-player practice run: the
 * judge holds up a tilted score card beside the drawing, and its feedback
 * follows. The card is the same one the duel reveal holds up, so a practice
 * score and a duel score read as the same thing measured. There is no
 * opponent, winner or Elo move. The feedback is the primary content, so it
 * gets the widest block and typography that survives the full 500 characters
 * the API allows. Presentational: PracticeView owns the run and every
 * navigation.
 */
import { computed } from 'vue'
import { OriButton, OriCard } from '@oriui/vue'
import { icons } from '@core'
import IslandSurface from '../../components/ui/IslandSurface.vue'

const props = defineProps<{
    /** Judge similarity, 0..1 exactly as the API delivers it. */
    score: number
    /** The judge's plain-text feedback (up to 500 characters). */
    feedback: string
    /** The prompt that was drawn, restated — after several seconds of judging the
     *  player has lost sight of it, and "Draw it again" needs it on screen. */
    prompt: string
    /** Advisory client-rendered PNG of the drawing (object URL), or null. Never
     *  what was judged — the authoritative raster is rendered server-side. */
    image: string | null
}>()

const emit = defineEmits<{ drawAgain: []; newPrompt: []; playDuel: [] }>()

/** Clamp to the 0..1 the contract promises — a display, not a validator. */
const clamped = computed(() => Math.max(0, Math.min(1, props.score)))
const percent = computed(() => Math.round(clamped.value * 100))

const band = computed(() => BANDS.find((b) => clamped.value >= b.min) ?? BANDS[BANDS.length - 1])
const headline = computed(() => band.value.headline)
/** Only the top band earns the primary colour, mirroring the duel reveal's winner. */
const topBand = computed(() => band.value === BANDS[0])
</script>

<template>
    <IslandSurface class="pr" role="dialog" aria-modal="false" aria-labelledby="pr-headline" elevation="lg">
        <header class="pr__head">
            <h2 id="pr-headline" class="pr__headline">{{ headline }}</h2>
            <p class="pr__prompt">You drew {{ prompt }}</p>
        </header>

        <div class="pr__scoreline">
            <div class="pr__canvas">
                <img v-if="image" :src="image" alt="Your drawing" />
                <span v-else class="pr__canvas-empty">No preview</span>
            </div>
            <OriCard
                class="pr__card"
                :variant="topBand ? 'soft' : 'outline'"
                :color="topBand ? 'primary' : 'surface'"
                radius="md"
            >
                <span class="pr__sr">{{ percent }} out of 100</span>
                <span class="pr__number" aria-hidden="true">{{ percent }}</span>
                <span class="pr__of" aria-hidden="true">out of 100</span>
            </OriCard>
        </div>

        <p class="pr__feedback">{{ feedback }}</p>

        <div class="pr__actions">
            <OriButton
                class="pr__action"
                label="Draw it again"
                variant="solid"
                color="primary"
                radius="md"
                fluid
                @click="emit('drawAgain')"
            />
            <OriButton
                class="pr__action"
                label="New prompt"
                variant="outline"
                color="surface"
                radius="md"
                fluid
                :icon="icons.target"
                icon-position="start"
                @click="emit('newPrompt')"
            />
        </div>

        <!-- Practice is the on-ramp, not the destination: once someone has drawn a
             prompt and been scored, they know how to duel. -->
        <OriButton
            class="pr__duel"
            label="Play a duel"
            variant="text"
            color="primary"
            radius="md"
            fluid
            :icon="icons.mdiSwordCross"
            icon-position="start"
            @click="emit('playDuel')"
        />
    </IslandSurface>
</template>

<style scoped>
/* Centred card in the shell's pointer-events:none overlay — opt back in so
   it's interactive. Scrolls internally: 500 characters of feedback plus the
   score row can outgrow a short phone, and feedback must never be what gets
   cut. */
.pr {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_lg, 0.75rem);

    width: min(94vw, 32rem);
    max-height: min(90dvh, 44rem);
    padding: var(--ori-size-gap_xl, 1rem);
    /* The card swings wider than the surface while it turns into place; that
       must not flash a horizontal scrollbar. */
    overflow: hidden auto;

    pointer-events: auto;
}

.pr__head {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);
}

.pr__headline {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: 1.3125rem;
    font-weight: 900;
    text-align: center;
}

.pr__prompt {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_md, 1rem);
    font-weight: 400;
    text-align: center;
    overflow-wrap: anywhere;
}

.pr__scoreline {
    display: grid;
    grid-template-columns: 8rem minmax(0, 11rem);
    gap: var(--ori-size-gap_xl, 1rem);
    align-items: center;
    justify-content: center;
}

/* A white square frame — the judged raster is rendered on white (GAME.md §6), so
   the thumbnail must not sit on a themed surface. */
.pr__canvas {
    display: grid;
    place-items: center;

    aspect-ratio: 1 / 1;
    overflow: hidden;

    border-radius: var(--ori-size-radius_sm, 4px);
    background-color: #ffffff;
}

.pr__canvas img {
    width: 100%;
    height: 100%;
    object-fit: contain;
}

.pr__canvas-empty {
    color: #444444;

    font-size: var(--ori-font-size_sm, 0.875rem);
    opacity: var(--jp-dim, 0.7);
}

/* The judge's card. The variant and colour props paint it; the turn and the
   entrance are ours, on the card's root. */
.pr__card {
    --ori-card-padding: var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_xl, 1rem);

    display: flex;
    flex-direction: column;
    align-items: center;

    width: 100%;

    color: var(--ori-color-on-surface);

    transform: rotate(2deg);

    animation: pr-held 360ms cubic-bezier(0.34, 1.56, 0.64, 1);
}

.pr__number {
    font-size: 3.75rem;
    font-weight: 1000;
    font-variant-numeric: tabular-nums;
    letter-spacing: -0.03em;
    line-height: 1;
}

.pr__of {
    font-size: var(--ori-font-size_sm, 0.875rem);
    font-weight: 700;
}

.pr__sr {
    position: absolute;

    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;

    border: 0;
    clip-path: inset(50%);
    white-space: nowrap;
}

.pr__feedback {
    margin: 0;
    padding: var(--ori-size-gap_md, 0.5rem) var(--ori-size-gap_lg, 0.75rem);

    border-radius: var(--ori-size-radius_sm, 4px);
    background-color: var(--ori-color-background);
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_md, 1rem);
    line-height: 1.5;
    /* A judge that returns one 500-character token would otherwise push the card
       wider than the viewport. */
    overflow-wrap: anywhere;
}

.pr__actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.pr__action {
    flex: 1 1 10rem;
}

/* The implicit end frame is the card's own tilt, so the overshoot settles on it. */
@keyframes pr-held {
    from {
        transform: translateY(1.25rem) rotate(8deg);
    }
}

@media (width <= 600px) {
    /* A phone gives the feedback a ~20rem column, so the full 500 characters run
       to a dozen-plus lines — step the type down and shrink the thumbnail so the
       card still opens on the feedback rather than scrolled past it. */
    .pr__scoreline {
        grid-template-columns: 6rem minmax(0, 11rem);
    }

    .pr__feedback {
        font-size: var(--ori-font-size_sm, 0.9rem);
        line-height: 1.55;
    }
}

@media (prefers-reduced-motion: reduce) {
    .pr__card {
        animation: none;
    }
}
</style>
