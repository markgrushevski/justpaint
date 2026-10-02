<script lang="ts">
/** One player's judged outcome — a 0..100 score and the rendered raster (or
 *  null: the opponent's canvas is fetched and rendered after the reveal opens,
 *  and a forfeiter has no submitted drawing to fetch at all). */
export interface DuelSide {
    /** Similarity score, 0..100 (clamped for display). */
    score: number
    /** A rendered PNG (object URL / data URI) of the drawing, or null. */
    image: string | null
}

/** The full judged result of a duel — everything the reveal screen needs. Shaped
 *  to mirror the server result (`MatchResultDone`), which PlayView maps straight
 *  into this. */
export interface DuelResult {
    you: DuelSide
    /** The opponent side plus their safe display label (never a login). */
    opponent: DuelSide & { name: string }
    /** Verdict from the judge, mapped to the local player (docs/GAME.md §7.1).
     *  `none` is not a draw — it means no verdict applies (the aborted case
     *  below), so an unscored round can never fall through into tie copy. */
    winner: 'you' | 'opponent' | 'tie' | 'none'
    /** The judge's reason string, shown verbatim. */
    reason: string
    /** How the match was decided (docs/GAME.md §3): `forfeit`/`aborted` mean
     *  no judge ran, so scores are meaningless — the reveal branches on this
     *  rather than comparing scores (docs/API.md §8, result). */
    resolution: 'judged' | 'forfeit' | 'aborted'
    /** Elo delta applied to the local player (may be negative). */
    eloDelta: number
    ratingBefore: number
}
</script>

<script lang="ts" setup>
/**
 * ResultReveal — the duel payoff screen: both drawings revealed side by side
 * (docs/GAME.md §4.2 — only once the match is done), each on its own tilted
 * score card that the judge holds up, then the judge's reason and the rating
 * move. The winner's card takes the primary colour. Presentational: PlayView
 * passes the result and owns "Play again".
 */
import { computed } from 'vue'
import { OriBadge, OriButton, OriCard, OriSurface } from '@oriui/vue'

const props = defineProps<{ result: DuelResult }>()
const emit = defineEmits<{ playAgain: []; viewLeaderboard: [] }>()

const youWon = computed(() => props.result.winner === 'you')
const tie = computed(() => props.result.winner === 'tie')
const winnerIsOpp = computed(() => props.result.winner === 'opponent')
const isForfeit = computed(() => props.result.resolution === 'forfeit')
const isAborted = computed(() => props.result.resolution === 'aborted')
/** Only a judged round has scores to show; the other two never ran the judge,
 *  so every score and rating move is suppressed rather than rendered as a
 *  truthful-looking 0. */
const scored = computed(() => props.result.resolution === 'judged')
const headline = computed(() => {
    // Neither of these ran the judge, so lead with what actually happened
    // rather than a normal win/lose framing (docs/API.md §8, result).
    if (isAborted.value) return 'Round couldn’t be scored'
    if (isForfeit.value) return youWon.value ? 'Opponent forfeited — you win' : 'You forfeited — no submission in time'
    return tie.value ? 'It’s a tie' : youWon.value ? 'You win!' : 'You lose'
})

const ratingAfter = computed(() => props.result.ratingBefore + props.result.eloDelta)
const deltaLabel = computed(() =>
    props.result.eloDelta >= 0 ? `+${props.result.eloDelta}` : `${props.result.eloDelta}`
)
const ratingLine = computed(() =>
    props.result.eloDelta === 0
        ? `Your rating stays at ${props.result.ratingBefore}`
        : `Your rating went from ${props.result.ratingBefore} to ${ratingAfter.value}`
)

/** Whole-number score for the card, clamped to 0..100. */
function scoreText(score: number): string {
    return String(Math.round(Math.max(0, Math.min(100, score))))
}

/** What one score card shows; `key` also picks which way it tilts. */
interface SideView {
    key: 'you' | 'opponent'
    name: string
    image: string | null
    alt: string
    score: string
    won: boolean
}

const sides = computed<SideView[]>(() => [
    {
        key: 'you',
        name: 'You',
        image: props.result.you.image,
        alt: 'Your drawing',
        score: scoreText(props.result.you.score),
        won: youWon.value
    },
    {
        key: 'opponent',
        name: props.result.opponent.name,
        image: props.result.opponent.image,
        alt: `${props.result.opponent.name}'s drawing`,
        score: scoreText(props.result.opponent.score),
        won: winnerIsOpp.value
    }
])
</script>

<template>
    <OriSurface
        class="result"
        role="dialog"
        aria-modal="false"
        aria-labelledby="result-headline"
        :bordered="false"
        elevation="lg"
    >
        <h2 id="result-headline" class="result__headline">{{ headline }}</h2>

        <div class="result__frames">
            <OriCard
                v-for="side in sides"
                :key="side.key"
                class="result__side"
                :class="`result__side--${side.key}`"
                :variant="side.won ? 'soft' : 'outline'"
                :color="side.won ? 'primary' : 'surface'"
                radius="md"
            >
                <div class="result__canvas">
                    <img v-if="side.image" :src="side.image" :alt="side.alt" />
                    <span v-else class="result__canvas-empty">No preview</span>
                </div>
                <p class="result__player">{{ side.name }}<span v-if="side.won" class="result__sr">, winner</span></p>
                <!-- No judge ran on a forfeit or an abort, so the score is null
                     server-side — a 0 would misread as a bad judged score. -->
                <template v-if="scored">
                    <span class="result__sr">{{ side.score }} out of 100</span>
                    <span class="result__number" aria-hidden="true">{{ side.score }}</span>
                    <span class="result__of" aria-hidden="true">out of 100</span>
                </template>
            </OriCard>
        </div>

        <p class="result__reason">{{ result.reason }}</p>

        <!-- A forfeit still moves the rating (full-K to the submitter), so the line
             stays for it; an aborted round moves nothing, and "1200 to 1200" reads
             as a result when it is really the absence of one. -->
        <div v-if="!isAborted" class="result__elo" role="group" aria-label="Rating change">
            <span class="result__rating">{{ ratingLine }}</span>
            <OriBadge
                v-if="result.eloDelta !== 0"
                class="result__delta"
                :content="deltaLabel"
                :color="result.eloDelta > 0 ? 'success' : 'danger'"
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
    </OriSurface>
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

.result__canvas-empty {
    color: #444444;

    font-size: var(--ori-font-size_sm, 0.875rem);
    opacity: 0.6;
}

.result__player {
    max-width: 100%;
    margin: var(--ori-size-gap_xs, 0.125rem) 0 0;
    overflow: hidden;

    font-size: var(--ori-font-size_md, 1rem);
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
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
