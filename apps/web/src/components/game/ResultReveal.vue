<script lang="ts">
/** One player's judged outcome — a 0..100 score and the rendered raster (or
 *  null, e.g. an opponent preview not yet wired). */
export interface DuelSide {
    /** Similarity score, 0..100 (clamped for display). */
    score: number
    /** A rendered PNG (object URL / data URI) of the drawing, or null. */
    image: string | null
}

/** The full judged result of a duel — everything the reveal screen needs. Shaped
 *  to mirror the eventual server result so real data drops straight in. */
export interface DuelResult {
    you: DuelSide
    /** The opponent side plus their safe display label (never a login). */
    opponent: DuelSide & { name: string }
    /** Verdict from the judge, mapped to the local player (GAME.md §7.1). */
    winner: 'you' | 'opponent' | 'tie'
    /** The judge's reason string, shown verbatim. */
    reason: string
    /** Elo delta applied to the local player (may be negative). */
    eloDelta: number
    /** The local player's rating before this match. */
    ratingBefore: number
}
</script>

<script lang="ts" setup>
/**
 * ResultReveal — the duel payoff screen: both canvases revealed side by side
 * (GAME.md §4.2 — only now, once the match is done), each with a 0–100% score
 * bar, a winner marker over the victor, the judge's reason, and an Elo pop
 * showing the rating move. Presentational: PlayView passes the result and owns
 * "Play again".
 */
import { computed } from 'vue'
import { OriButton, OriCard, OriSurface } from '@oriui/vue'

const props = defineProps<{ result: DuelResult }>()
const emit = defineEmits<{ playAgain: [] }>()

const youWon = computed(() => props.result.winner === 'you')
const tie = computed(() => props.result.winner === 'tie')
const winnerIsOpp = computed(() => props.result.winner === 'opponent')
const headline = computed(() => (tie.value ? 'It’s a tie' : youWon.value ? 'You win!' : 'You lose'))

const ratingAfter = computed(() => props.result.ratingBefore + props.result.eloDelta)
const deltaLabel = computed(() =>
    props.result.eloDelta >= 0 ? `+${props.result.eloDelta}` : `${props.result.eloDelta}`
)

/** Clamp a raw score to a 0..100% bar width. */
function pct(score: number): string {
    return `${Math.max(0, Math.min(100, score))}%`
}
/** Whole-number score for the label. */
function scoreText(score: number): string {
    return String(Math.round(Math.max(0, Math.min(100, score))))
}
</script>

<template>
    <OriSurface class="result" role="dialog" aria-modal="false" aria-labelledby="result-headline">
        <h2
            id="result-headline"
            class="result__headline"
            :class="{ 'result__headline--win': youWon, 'result__headline--tie': tie }"
        >
            {{ headline }}
        </h2>

        <div class="result__frames">
            <!-- You -->
            <OriCard
                class="result__side"
                :variant="youWon ? 'tonal' : 'outline'"
                :color="youWon ? 'primary' : 'surface'"
                radius="md"
            >
                <span v-if="youWon" class="result__crown" aria-label="Winner">▲ Winner</span>
                <div class="result__canvas">
                    <img v-if="result.you.image" :src="result.you.image" alt="Your drawing" />
                    <span v-else class="result__canvas-empty">No preview</span>
                </div>
                <div class="result__meta">
                    <span class="result__player">You</span>
                    <span class="result__score">{{ scoreText(result.you.score) }}%</span>
                </div>
                <div class="result__bar">
                    <div class="result__bar-fill result__bar-fill--you" :style="{ width: pct(result.you.score) }"></div>
                </div>
            </OriCard>

            <!-- Opponent -->
            <OriCard
                class="result__side"
                :variant="winnerIsOpp ? 'tonal' : 'outline'"
                :color="winnerIsOpp ? 'primary' : 'surface'"
                radius="md"
            >
                <span v-if="winnerIsOpp" class="result__crown" aria-label="Winner">▲ Winner</span>
                <div class="result__canvas">
                    <img
                        v-if="result.opponent.image"
                        :src="result.opponent.image"
                        :alt="`${result.opponent.name}'s drawing`"
                    />
                    <span v-else class="result__canvas-empty">No preview</span>
                </div>
                <div class="result__meta">
                    <span class="result__player">{{ result.opponent.name }}</span>
                    <span class="result__score">{{ scoreText(result.opponent.score) }}%</span>
                </div>
                <div class="result__bar">
                    <div
                        class="result__bar-fill result__bar-fill--opp"
                        :style="{ width: pct(result.opponent.score) }"
                    ></div>
                </div>
            </OriCard>
        </div>

        <p class="result__reason">
            <span class="result__reason-label">Judge</span>
            {{ result.reason }}
        </p>

        <div class="result__elo" role="group" aria-label="Rating change">
            <span class="result__rating">{{ result.ratingBefore }}</span>
            <span class="result__arrow" aria-hidden="true">→</span>
            <span class="result__rating result__rating--after">{{ ratingAfter }}</span>
            <span class="result__delta" :class="result.eloDelta >= 0 ? 'result__delta--up' : 'result__delta--down'">
                {{ deltaLabel }}
            </span>
        </div>

        <OriButton
            class="result__again"
            text="Play again"
            variant="fill"
            color="primary"
            radius="md"
            fluid
            @click="emit('playAgain')"
        />
    </OriSurface>
</template>

<style scoped>
/* Centred card in the shell's pointer-events:none overlay — opt back in so the
   card is interactive. Scrolls internally on short viewports. */
.result {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_md, 0.5rem);

    width: min(94vw, 34rem);
    max-height: min(90dvh, 44rem);
    padding: var(--ori-size-gap_lg, 0.75rem);
    overflow-y: auto;

    pointer-events: auto;

    animation: result-pop 0.24s ease-out;
}

.result__headline {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_xl, 1.4rem);
    font-weight: 800;
    letter-spacing: -0.01em;
    text-align: center;
}

.result__headline--win {
    color: var(--ori-color-primary);
}

.result__headline--tie {
    opacity: 0.85;
}

.result__frames {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--ori-size-gap_md, 0.5rem);
}

.result__side {
    /* Winner tint is now owned by OriCard's variant/color props (tonal+primary vs
       outline+surface) — no local border/background here. A hardcoded border would
       double up with OriCard's own variant border and always win (unlayered component
       styles beat oriui's @layer rules), silently forcing the outline variant transparent. */
    position: relative;

    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);

    padding: var(--ori-size-gap_sm, 0.25rem);

    border-radius: var(--ori-size-radius_md, 8px);
}

.result__crown {
    position: absolute;
    top: -0.6rem;
    left: 50%;
    transform: translateX(-50%);
    z-index: 1;

    padding: 0.05rem 0.5rem;

    border-radius: var(--ori-size-radius_rounded, 999px);
    background-color: var(--ori-color-primary);
    color: var(--ori-color-on-primary);

    font-size: var(--ori-font-size_xs, 0.7rem);
    font-weight: 800;
    white-space: nowrap;
}

.result__canvas {
    display: grid;
    place-items: center;

    aspect-ratio: 1 / 1;
    overflow: hidden;

    border-radius: var(--ori-size-radius_sm, 4px);
    /* An opaque white frame — the judged raster is rendered on white (GAME.md §6). */
    background-color: #ffffff;
}

.result__canvas img {
    width: 100%;
    height: 100%;
    object-fit: contain;
}

.result__canvas-empty {
    color: #444444;

    font-size: var(--ori-font-size_xs, 0.75rem);
    opacity: 0.6;
}

.result__meta {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.result__player {
    overflow: hidden;

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.result__score {
    flex: none;

    font-size: var(--ori-font-size_sm, 0.9rem);
    font-weight: 800;
    font-variant-numeric: tabular-nums;
}

.result__bar {
    height: 0.5rem;
    overflow: hidden;

    border-radius: var(--ori-size-radius_rounded, 999px);
    background-color: color-mix(in srgb, var(--ori-color-on-surface) 12%, transparent);
}

.result__bar-fill {
    height: 100%;

    border-radius: inherit;
    transition: width 0.5s ease-out;
}

.result__bar-fill--you {
    background-color: var(--ori-color-primary);
}

.result__bar-fill--opp {
    background-color: var(--ori-color-secondary);
}

.result__reason {
    margin: 0;
    padding: var(--ori-size-gap_sm, 0.25rem) var(--ori-size-gap_md, 0.5rem);

    border-left: 3px solid var(--ori-color-primary);
    border-radius: var(--ori-size-radius_sm, 4px);
    background-color: var(--ori-color-background);

    font-size: var(--ori-font-size_sm, 0.875rem);
    line-height: 1.4;
}

.result__reason-label {
    display: inline-block;
    margin-right: 0.4rem;

    color: var(--ori-color-primary);

    font-size: var(--ori-font-size_xs, 0.7rem);
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
}

.result__elo {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    font-variant-numeric: tabular-nums;
}

.result__rating {
    font-size: var(--ori-font-size_md, 1rem);
    font-weight: 700;
    opacity: 0.7;
}

.result__rating--after {
    font-size: var(--ori-font-size_lg, 1.15rem);
    opacity: 1;
}

.result__arrow {
    opacity: 0.6;
}

.result__delta {
    padding: 0.05rem 0.45rem;

    border-radius: var(--ori-size-radius_rounded, 999px);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-weight: 800;

    animation: result-pop 0.4s ease-out 0.15s both;
}

.result__delta--up {
    background-color: color-mix(in srgb, var(--ori-color-success) 22%, transparent);
    color: var(--ori-color-success-text, var(--ori-color-success));
}

.result__delta--down {
    background-color: color-mix(in srgb, var(--ori-color-danger) 22%, transparent);
    color: var(--ori-color-danger-text, var(--ori-color-danger));
}

.result__again {
    margin-top: var(--ori-size-gap_xs, 0.125rem);
}

@keyframes result-pop {
    from {
        opacity: 0;
        transform: scale(0.96);
    }
}

@media (prefers-reduced-motion: reduce) {
    .result,
    .result__delta {
        animation: none;
    }

    .result__bar-fill {
        transition: none;
    }
}
</style>
