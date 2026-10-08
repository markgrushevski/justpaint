<script lang="ts" setup>
/**
 * ResultReveal — the duel payoff screen: both drawings revealed side by side
 * (docs/GAME.md §4.2 — only once the match is done), each on its own tilted
 * score card that the judge holds up, then the judge's reason and the rating
 * move. The winner's card takes the primary colour. Presentational: PlayView
 * passes the result, worded for the viewer (`toDuelResult`), and owns "Play again".
 */
import { computed, onMounted, ref } from 'vue'
import { OriBadge, OriButton, OriCard, OriSkeleton } from '@oriui/vue'
import IslandSurface from '../../components/ui/IslandSurface.vue'
import type { DuelResult, DuelSide } from './duel'

const props = defineProps<{
    result: DuelResult
    /** Rendered drawings (object URLs); null while loading or when there is none. */
    images: { you: string | null; opponent: string | null }
    loading: boolean
}>()
const emit = defineEmits<{ playAgain: []; viewLeaderboard: [] }>()

const delta = computed(() => (props.result.rating ? props.result.rating.after - props.result.rating.before : 0))
const deltaLabel = computed(() => (delta.value >= 0 ? `+${delta.value}` : `${delta.value}`))
const ratingLine = computed(() => {
    const rating = props.result.rating
    if (!rating) return ''
    return delta.value === 0
        ? `Your rating stays at ${rating.before}`
        : `Your rating went from ${rating.before} to ${rating.after}`
})

/** Whole-number score for the card, clamped to 0..100. */
function scoreText(score: number | null): string {
    return String(Math.round(Math.max(0, Math.min(100, score ?? 0))))
}

interface SideView {
    key: 'you' | 'opponent'
    side: DuelSide
    image: string | null
    alt: string
    /** Shown in place of a picture. */
    empty: string
}

const sides = computed<SideView[]>(() => {
    const view = (key: 'you' | 'opponent', side: DuelSide, image: string | null, alt: string): SideView => ({
        key,
        side,
        image,
        alt,
        empty: side.drew ? 'No preview' : 'No drawing'
    })
    return [
        view('you', props.result.you, props.images.you, 'Your drawing'),
        view('opponent', props.result.opponent, props.images.opponent, `${props.result.opponent.name}'s drawing`)
    ]
})

// Focus lands on the verdict, so a keyboard or screen-reader player starts there.
const headline = ref<HTMLHeadingElement | null>(null)
onMounted(() => headline.value?.focus())
</script>

<template>
    <IslandSurface class="result" role="dialog" aria-modal="false" aria-labelledby="result-headline" elevation="lg">
        <h2 id="result-headline" ref="headline" class="result__headline" tabindex="-1">{{ result.headline }}</h2>

        <div class="result__frames">
            <OriCard
                v-for="view in sides"
                :key="view.key"
                class="result__side"
                :class="`result__side--${view.key}`"
                :variant="view.side.won ? 'soft' : 'outline'"
                :color="view.side.won ? 'primary' : 'surface'"
                radius="md"
            >
                <div class="result__canvas">
                    <img v-if="view.image" :src="view.image" :alt="view.alt" />
                    <OriSkeleton v-else-if="loading && view.side.drew" class="result__canvas-loading" />
                    <span v-else class="result__canvas-empty">{{ view.empty }}</span>
                </div>
                <p class="result__player">
                    <span class="result__name">{{ view.side.name }}</span>
                    <!-- The judge's reason names the drawings by these letters. -->
                    <OriBadge
                        v-if="view.side.letter"
                        class="result__letter"
                        :content="view.side.letter"
                        :label="`drawing ${view.side.letter}`"
                        color="surface"
                        variant="outline"
                    />
                    <span v-if="view.side.won" class="result__sr">, winner</span>
                </p>
                <!-- No judge ran on a forfeit or an abort, so the score is null
                     server-side — a 0 would misread as a bad judged score. -->
                <template v-if="result.scored">
                    <span class="result__sr">{{ scoreText(view.side.score) }} out of 100</span>
                    <span class="result__number" aria-hidden="true">{{ scoreText(view.side.score) }}</span>
                    <span class="result__of" aria-hidden="true">out of 100</span>
                </template>
            </OriCard>
        </div>

        <p v-if="result.reason" class="result__reason">{{ result.reason }}</p>

        <!-- A forfeit still moves the rating (full-K to the submitter), so the line
             stays for it; an aborted round moves nothing, and "1200 to 1200" reads
             as a result when it is really the absence of one. -->
        <div v-if="result.rating" class="result__elo" role="group" aria-label="Rating change">
            <span class="result__rating">{{ ratingLine }}</span>
            <OriBadge
                v-if="delta !== 0"
                class="result__delta"
                :content="deltaLabel"
                :color="delta > 0 ? 'success' : 'danger'"
                variant="soft"
            />
        </div>

        <!-- Post-duel is peak intent to check standings — the only path a /play
             user reaches the ladder (PlayView has no drawer). Presentational: emit
             and let PlayView route. -->
        <div class="result__actions">
            <OriButton
                class="result__again"
                label="Play again"
                variant="solid"
                color="primary"
                radius="md"
                fluid
                @click="emit('playAgain')"
            />
            <OriButton
                class="result__leaderboard"
                label="View leaderboard"
                variant="outline"
                color="surface"
                radius="md"
                fluid
                @click="emit('viewLeaderboard')"
            />
        </div>
    </IslandSurface>
</template>

<style scoped>
/* Centred card in the shell's pointer-events:none overlay — opt back in so the
   card is interactive. Scrolls internally on short viewports. */
.result {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_lg, 0.75rem);

    width: min(94vw, 34rem);
    max-height: min(90dvh, 44rem);
    padding: var(--ori-size-gap_xl, 1rem);
    /* The cards swing wider than the surface while they turn into place; that
       must not flash a horizontal scrollbar. */
    overflow: hidden auto;

    pointer-events: auto;
}

.result__headline {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: 1.3125rem;
    font-weight: 900;
    text-align: center;
}

/* Focused by script only, to start reading there; it is not a control. */
.result__headline:focus {
    outline: none;
}

/* The gap leaves room for the tilt: each corner swings a few pixels out. */
.result__frames {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--ori-size-gap_xl, 1rem);

    padding-block: var(--ori-size-gap_sm, 0.25rem);
}

/* The winner's tint is owned by OriCard's variant/color props; the turn and the
   entrance are ours, on the card's root. */
.result__side {
    --ori-card-padding: var(--ori-size-gap_md, 0.5rem);
    --result-tilt: -2deg;

    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_xs, 0.125rem);

    color: var(--ori-color-on-surface);

    transform: rotate(var(--result-tilt));

    animation: result-held 360ms cubic-bezier(0.34, 1.56, 0.64, 1) backwards;
}

/* The judge holds the second card up a beat after the first. */
.result__side--opponent {
    --result-tilt: 2deg;

    animation-delay: 90ms;
}

.result__canvas {
    display: grid;
    place-items: center;

    width: 100%;
    aspect-ratio: 1 / 1;
    overflow: hidden;

    border-radius: var(--ori-size-radius_sm, 4px);
    /* An opaque white frame — the judged raster is rendered on white (docs/GAME.md §6). */
    background-color: #ffffff;
}

.result__canvas img {
    width: 100%;
    height: 100%;
    object-fit: contain;
}

.result__canvas-loading {
    width: 100%;
    height: 100%;
}

.result__canvas-empty {
    color: #444444;

    font-size: var(--ori-font-size_sm, 0.875rem);
    opacity: var(--jp-dim, 0.7);
}

.result__player {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    max-width: 100%;
    margin: var(--ori-size-gap_xs, 0.125rem) 0 0;

    font-size: var(--ori-font-size_md, 1rem);
    font-weight: 700;
}

.result__name {
    min-width: 0;
    overflow: hidden;

    text-overflow: ellipsis;
    white-space: nowrap;
}

.result__letter {
    flex: none;

    font-size: var(--ori-font-size_xs, 0.75rem);
    font-weight: 800;
}

.result__number {
    font-size: 3.75rem;
    font-weight: 1000;
    font-variant-numeric: tabular-nums;
    letter-spacing: -0.03em;
    line-height: 1;
}

.result__of {
    font-size: var(--ori-font-size_sm, 0.875rem);
    font-weight: 700;
}

.result__sr {
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

.result__reason {
    margin: 0;
    padding: var(--ori-size-gap_md, 0.5rem) var(--ori-size-gap_lg, 0.75rem);

    border-radius: var(--ori-size-radius_sm, 4px);
    background-color: var(--ori-color-background);
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_md, 1rem);
    line-height: 1.5;
    overflow-wrap: anywhere;
}

.result__elo {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: center;
    gap: var(--ori-size-gap_md, 0.5rem);

    font-variant-numeric: tabular-nums;
}

.result__rating {
    font-size: var(--ori-font-size_md, 1rem);
    font-weight: 700;
}

.result__delta {
    font-size: var(--ori-font-size_sm, 0.875rem);
    font-weight: 700;
}

.result__actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.result__again,
.result__leaderboard {
    flex: 1 1 10rem;
}

/* The implicit end frame is the card's own tilt, so the overshoot settles on it. */
@keyframes result-held {
    from {
        transform: translateY(1.25rem) rotate(calc(var(--result-tilt) * 3));
    }
}

@media (width <= 600px) {
    /* Clears the prompt pill above the centred card. */
    .result {
        margin-top: 2.5rem;
    }

    .result__reason {
        font-size: var(--ori-font-size_sm, 0.875rem);
        line-height: 1.55;
    }
}

@media (prefers-reduced-motion: reduce) {
    .result__side {
        animation: none;
    }
}
</style>
