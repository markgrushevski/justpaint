<script lang="ts" setup>
/**
 * JudgingOverlay — the pending state shown between submit and result: a centred
 * skeleton card that foreshadows the result layout while the server renders
 * authoritative rasters and awaits the judge (GAME.md §4, `judging`).
 *
 * `solo` switches it to the single-player practice wait: one skeleton frame and
 * copy that doesn't mention an opponent. The two modes share this component
 * because the moment IS the same one — the judge is looking at a drawing — and a
 * second copy of the scrim would drift; the only thing practice must not do is
 * render an opponent frame for a player who has none.
 *
 * Presentational: the view shows it while its own phase says judging. It renders
 * a full-bleed scrim (pointer-events:auto) so stray taps don't reach the canvas
 * mid-judging; the shell's overlay layer is pointer-events:none, so opting back
 * in here is required.
 */
import { OriSkeleton, OriSpinner, OriSurface } from '@oriui/vue'

withDefaults(defineProps<{ opponentName?: string; solo?: boolean }>(), {
    opponentName: 'Player 2',
    solo: false
})
</script>

<template>
    <div class="judging">
        <OriSurface class="judging__card">
            <OriSpinner size="lg" color="primary" />
            <h2 class="judging__title">{{ solo ? 'The judge is looking…' : 'Judging the duel…' }}</h2>
            <p class="judging__sub">
                {{ solo ? 'Scoring your drawing against the prompt.' : 'Scoring both drawings against the prompt.' }}
            </p>

            <!-- One frame solo, two in a duel: the grid narrows to a single column
                 so the lone skeleton doesn't sit beside an empty half. -->
            <div class="judging__frames" :class="{ 'judging__frames--solo': solo }">
                <div class="judging__frame">
                    <OriSkeleton class="judging__canvas" radius="md" />
                    <span class="judging__cap">You</span>
                </div>
                <div v-if="!solo" class="judging__frame">
                    <OriSkeleton class="judging__canvas" radius="md" />
                    <span class="judging__cap">{{ opponentName }}</span>
                </div>
            </div>
        </OriSurface>
    </div>
</template>

<style scoped>
/* Full-bleed scrim within the shell's pointer-events:none overlay layer — opt
   back into pointer events so the canvas is inert while judging. */
.judging {
    position: absolute;
    inset: 0;

    display: grid;
    place-items: center;

    background-color: color-mix(in srgb, var(--ori-color-background) 55%, transparent);
    backdrop-filter: blur(2px);
    pointer-events: auto;
}

.judging__card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: min(92vw, 30rem);
    padding: var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_lg, 0.75rem) var(--ori-size-gap_xl, 1rem);

    text-align: center;
}

.judging__title {
    margin: var(--ori-size-gap_sm, 0.25rem) 0 0;

    font-size: var(--ori-font-size_lg, 1.15rem);
    font-weight: 800;
    letter-spacing: -0.01em;
}

.judging__sub {
    margin: 0 0 var(--ori-size-gap_md, 0.5rem);

    font-size: var(--ori-font-size_sm, 0.85rem);
    opacity: 0.7;
}

.judging__frames {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--ori-size-gap_md, 0.5rem);

    width: 100%;
}

/* Solo: one frame, centred and held to half the card so a single square doesn't
   stretch to a wall of skeleton. */
.judging__frames--solo {
    grid-template-columns: minmax(0, 1fr);

    width: 50%;
    margin: 0 auto;
}

.judging__frame {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);
    align-items: center;
}

.judging__canvas {
    width: 100%;
    /* Square, echoing the 1080² duel canvas the result will show. */
    aspect-ratio: 1 / 1;
}

.judging__cap {
    font-size: var(--ori-font-size_xs, 0.75rem);
    font-weight: 700;
    opacity: 0.7;
}
</style>
