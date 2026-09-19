<script lang="ts">
/**
 * Score bands, highest first. The judge hands back a number; a number alone is
 * not a verdict, so the card leads with the band and lets the percentage back it
 * up — the same reason the duel leads with "You win!" rather than "72% vs 64%".
 * The thresholds are copy, not contract: nothing downstream reads them.
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
 * PracticeResult — the payoff screen for a single-player practice run. It wears
 * the duel reveal's visual language on purpose (the same 0–100% bar in the same
 * primary fill, the same white square canvas frame, the same card chrome) so a
 * practice score and a duel score read as the same thing being measured.
 *
 * What it deliberately does NOT have: an opponent, a winner, an Elo move. There
 * is one drawing and nobody to beat, and inventing any of the three would be a
 * lie the duel screen at least earns.
 *
 * The FEEDBACK is the point. In a duel the judge's reason explains who won; here
 * it is the entire product — the thing a player comes back for — so it gets the
 * widest block on the card, room to breathe, and typography that survives the
 * full 500 characters the contract allows on a phone.
 *
 * Presentational: PracticeView owns the run and every navigation.
 */
import { computed } from 'vue'
import { OriButton, OriSurface } from '@oriui/vue'
import { icons } from '@core'

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
const barWidth = computed(() => `${percent.value}%`)

const band = computed(() => BANDS.find((b) => clamped.value >= b.min) ?? BANDS[BANDS.length - 1])
const headline = computed(() => band.value.headline)
/** Only the top band earns the accent, mirroring the duel reveal's win colour. */
const topBand = computed(() => band.value === BANDS[0])
</script>

<template>
    <OriSurface class="pr" role="dialog" aria-modal="false" aria-labelledby="pr-headline">
        <h2 id="pr-headline" class="pr__headline" :class="{ 'pr__headline--top': topBand }">{{ headline }}</h2>

        <p class="pr__prompt">
            <span class="pr__prompt-label">Prompt</span>
            {{ prompt }}
        </p>

        <div class="pr__scoreline">
            <div class="pr__canvas">
                <img v-if="image" :src="image" alt="Your drawing" />
                <span v-else class="pr__canvas-empty">No preview</span>
            </div>
            <div class="pr__score">
                <span class="pr__percent">{{ percent }}%</span>
                <div class="pr__bar">
                    <div class="pr__bar-fill" :style="{ width: barWidth }"></div>
                </div>
                <!-- The duel's two bars explain themselves by comparison; a lone
                     bar has to say what it is measuring. -->
                <span class="pr__caption">similarity to the prompt</span>
            </div>
        </div>

        <div class="pr__feedback">
            <span class="pr__feedback-label">Judge</span>
            <p class="pr__feedback-text">{{ feedback }}</p>
        </div>

        <div class="pr__actions">
            <OriButton
                class="pr__action"
                text="Draw it again"
                variant="fill"
                color="primary"
                radius="md"
                fluid
                @click="emit('drawAgain')"
            />
            <OriButton
                class="pr__action"
                text="New prompt"
                variant="outline"
                color="surface"
                radius="md"
                fluid
                :icon="icons.target"
                icon-position="left"
                @click="emit('newPrompt')"
            />
        </div>

        <!-- Practice is the on-ramp, not the destination: once someone has drawn a
             prompt and been scored, they know how to duel. -->
        <OriButton
            class="pr__duel"
            text="Play a duel"
            variant="text"
            color="primary"
            radius="md"
            fluid
            :icon="icons.mdiSwordCross"
            icon-position="left"
            @click="emit('playDuel')"
        />
    </OriSurface>
</template>

<style scoped>
/* Centred card in the shell's pointer-events:none overlay — opt back in so the
   card is interactive. Scrolls internally: 500 characters of feedback plus the
   score row will outgrow a short phone, and the feedback must never be the thing
   that gets cut. */
.pr {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);

    width: min(94vw, 32rem);
    max-height: min(90dvh, 44rem);
    padding: var(--ori-size-gap_lg, 0.75rem);
    overflow-y: auto;

    pointer-events: auto;

    animation: pr-pop 0.24s ease-out;
}

.pr__headline {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xl, 1.4rem);
    font-weight: 800;
    letter-spacing: -0.01em;
    text-align: center;
}

.pr__headline--top {
    color: var(--ori-color-primary);
}

.pr__prompt {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_md, 1rem);
    font-weight: 700;
    text-align: center;
    overflow-wrap: anywhere;
}

.pr__prompt-label {
    display: inline-block;
    margin-right: 0.4rem;

    color: var(--ori-color-primary);

    font-size: var(--ori-font-size_xs, 0.7rem);
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
}

.pr__scoreline {
    display: grid;
    grid-template-columns: 8rem minmax(0, 1fr);
    gap: var(--ori-size-gap_md, 0.5rem);
    align-items: center;
}

/* The same white square frame as the duel reveal — the judged raster is rendered
   on white (GAME.md §6), so the thumbnail must not sit on a themed surface. */
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

    font-size: var(--ori-font-size_xs, 0.75rem);
    opacity: 0.6;
}

.pr__score {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);
}

.pr__percent {
    font-size: var(--ori-font-size_xl, 1.6rem);
    font-weight: 800;
    font-variant-numeric: tabular-nums;
    line-height: 1.1;
}

/* Bar geometry copied from the duel reveal on purpose (same height, same track,
   same primary fill): two screens measuring the same thing should look it. */
.pr__bar {
    height: 0.5rem;
    overflow: hidden;

    border-radius: var(--ori-size-radius_rounded, 999px);
    background-color: color-mix(in srgb, var(--ori-color-on-surface) 12%, transparent);
}

.pr__bar-fill {
    height: 100%;

    border-radius: inherit;
    background-color: var(--ori-color-primary);
    transition: width 0.5s ease-out;
}

.pr__caption {
    font-size: var(--ori-font-size_xs, 0.75rem);
    opacity: 0.7;
}

/* The hero block. Same accent rule as the duel's reason so the two read as one
   voice, but full width and with room — this is what the player came for. */
.pr__feedback {
    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);

    border-left: 3px solid var(--ori-color-primary);
    border-radius: var(--ori-size-radius_sm, 4px);
    background-color: var(--ori-color-background);
}

.pr__feedback-label {
    display: block;

    color: var(--ori-color-primary);

    font-size: var(--ori-font-size_xs, 0.7rem);
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
}

.pr__feedback-text {
    margin: 0.2rem 0 0;

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
    gap: var(--ori-size-gap_sm, 0.25rem);

    margin-top: var(--ori-size-gap_xs, 0.125rem);
}

.pr__action {
    /* Share the row evenly; wrap to full width on a very narrow card. */
    flex: 1 1 10rem;
}

@keyframes pr-pop {
    from {
        opacity: 0;
        transform: scale(0.96);
    }
}

@media (width <= 600px) {
    /* A phone gives the feedback a ~20rem column, so the full 500 characters run
       to a dozen-plus lines — step the type down and shrink the thumbnail so the
       card still opens on the feedback rather than scrolled past it. */
    .pr__scoreline {
        grid-template-columns: 6rem minmax(0, 1fr);
    }

    .pr__feedback-text {
        font-size: var(--ori-font-size_sm, 0.9rem);
        line-height: 1.55;
    }
}

@media (prefers-reduced-motion: reduce) {
    .pr {
        animation: none;
    }

    .pr__bar-fill {
        transition: none;
    }
}
</style>
