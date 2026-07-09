<script lang="ts" setup>
/**
 * JudgingOverlay — the pending state shown between submit and result: a centred
 * skeleton card that foreshadows the two-canvas result layout while the server
 * renders authoritative rasters and awaits the judge (GAME.md §4, `judging`).
 *
 * Presentational: PlayView shows it while `phase = 'judging'`. It renders a
 * full-bleed scrim (pointer-events:auto) so stray taps don't reach the canvas
 * mid-judging; the shell's overlay layer is pointer-events:none, so opting back
 * in here is required.
 */
import { OriSkeleton, OriSpinner } from '@oriui/vue'

withDefaults(defineProps<{ opponentName?: string }>(), { opponentName: 'Player 2' })
</script>

<template>
    <div class="judging">
        <div class="judging__card jp-float">
            <OriSpinner size="lg" color="primary" />
            <h2 class="judging__title">Judging the duel…</h2>
            <p class="judging__sub">Scoring both drawings against the prompt.</p>

            <div class="judging__frames">
                <div class="judging__frame">
                    <OriSkeleton class="judging__canvas" radius="md" />
                    <span class="judging__cap">You</span>
                </div>
                <div class="judging__frame">
                    <OriSkeleton class="judging__canvas" radius="md" />
                    <span class="judging__cap">{{ opponentName }}</span>
                </div>
            </div>
        </div>
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
