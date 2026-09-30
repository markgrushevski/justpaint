<script lang="ts">
/**
 * Confidence bands, highest first: the endpoint returns 0..1, but a raw
 * number reads as false precision, so the card renders a phrase instead.
 * Copy, not contract — nothing downstream reads these thresholds.
 */
const BANDS: { min: number; text: string }[] = [
    { min: 0.85, text: 'I’d bet on it.' },
    { min: 0.6, text: 'Fairly sure about that one.' },
    { min: 0.35, text: 'Not too sure, though.' },
    { min: 0, text: 'Honestly, that’s a wild guess.' }
]

/**
 * What the card is showing — a single discriminant (mirrors PracticeView's
 * `Phase`), so the template's status chain is exhaustive by construction.
 *
 * `idle` is nothing asked yet (today: a blank canvas) — the card says so
 * itself, since a disabled button can't speak on touch. `exhausted` stays
 * beside `error` rather than folded into it: the two are orthogonal.
 */
export type GuessStatus = 'idle' | 'pending' | 'answered' | 'error'

/**
 * The action's label, one line per status. A `Record` over the union, not a
 * chain of truthiness checks, so adding a status is a compile error here
 * instead of a silently blank button, and needs no unreachable default to
 * satisfy `vue/return-in-computed-property`.
 */
const ACTION_TEXT: Record<GuessStatus, string> = {
    idle: 'Guess my drawing',
    pending: 'Thinking…',
    answered: 'Guess again',
    error: 'Try again'
}
</script>

<script lang="ts" setup>
/**
 * GuessResult — the single surface for the AI-guess feature: the wait, the
 * answer and every failure all render here rather than in a toast, since the
 * daily-cap message deserves more than toast styling.
 *
 * Presentational only: props in, events out, no API calls — DrawView owns the
 * request, the budget and the canvas.
 */
import { computed } from 'vue'
import { OriButton, OriSurface } from '@oriui/vue'
import IconButton from '../../components/ui/IconButton.vue'

const props = withDefaults(
    defineProps<{
        /** The discriminant this card renders on. */
        status?: GuessStatus
        /** The AI's best reading; read only while `status === 'answered'`. */
        label?: string | null
        /** Its certainty, 0..1 — rendered as a phrase, never as the number. */
        confidence?: number
        /** 0–2 runner-up guesses; the "or maybe" line. */
        alternatives?: string[]
        /** A failure message, in the server's own words; read only on `error`. */
        error?: string
        /**
         * The spent daily cap, not a fault or a per-IP rate limit that clears in
         * seconds. Qualifies `error`; drops the retry since it can't succeed today.
         */
        exhausted?: boolean
        /** False when there is nothing to ask about (a blank canvas), which
         *  disables the action rather than spending a call on an empty page. */
        canRetry?: boolean
    }>(),
    {
        status: 'idle',
        label: null,
        confidence: 0,
        alternatives: () => [],
        error: '',
        exhausted: false,
        canRetry: true
    }
)

const emit = defineEmits<{ again: []; dismiss: [] }>()

/** Clamp to the 0..1 the contract promises — a display, not a validator. */
const clamped = computed(() => Math.max(0, Math.min(1, props.confidence)))
const confidenceText = computed(() => (BANDS.find((b) => clamped.value >= b.min) ?? BANDS[BANDS.length - 1]).text)

/**
 * The action doubles as the pending indicator: `loading` gives it oriui's
 * spinner plus `aria-busy`, so the wait shows on the control that caused it.
 */
const actionText = computed(() => ACTION_TEXT[props.status])

/** `loading` + `disabled` both key off the wait; naming it keeps the template honest. */
const pending = computed(() => props.status === 'pending')
</script>

<template>
    <OriSurface class="guess" role="group" aria-labelledby="guess-title">
        <!-- Explicit close: the trigger that opened this card can be off-screen
             on a narrow phone, so it must stay dismissible from within. -->
        <div class="guess__head">
            <span id="guess-title" class="guess__title">AI guess</span>
            <IconButton icon="close" label="Dismiss the guess" placement="bottom" @click="emit('dismiss')" />
        </div>

        <!-- role=status is an implicit polite live region: the answer lands
             seconds after focus has moved on, so a screen reader must be told.
             One arm per `GuessStatus`, ending in a bare v-else, so the chain is
             exhaustive by construction. -->
        <div class="guess__body" role="status">
            <template v-if="status === 'pending'">
                <p class="guess__lead">Looking at your drawing…</p>
                <p class="guess__quiet">It gets redrawn on the server first, so this takes a few seconds.</p>
            </template>

            <template v-else-if="status === 'error'">
                <p class="guess__lead">{{ exhausted ? 'That’s your guessing for today' : 'The AI didn’t answer' }}</p>
                <!-- The server's own wording, verbatim: only it knows whether the
                     visitor spent their guesses or the service spent its budget,
                     and it says the first without leaking the second. -->
                <p class="guess__msg">{{ error }}</p>
            </template>

            <template v-else-if="status === 'answered'">
                <p class="guess__lead">
                    I think this is <strong class="guess__label">{{ label }}</strong>
                </p>
                <p class="guess__quiet">{{ confidenceText }}</p>
                <!-- Runner-ups are secondary to the headline guess: quiet, one line. -->
                <p v-if="alternatives.length > 0" class="guess__alts">or maybe: {{ alternatives.join(', ') }}</p>
            </template>

            <!-- idle: nothing asked yet. The reason lives here, not in the
                 trigger's tooltip — oriui's tooltip needs hover/focus-within, so
                 on a phone a disabled trigger would show no reason at all. -->
            <template v-else>
                <p class="guess__lead">Draw something first</p>
                <p class="guess__quiet">Then I can tell you what I think it is.</p>
            </template>
        </div>

        <!-- Secondary styling, sized to its own text: a full-bleed primary bar
             would be the loudest thing on screen for spending 1 of only 2 daily
             calls. A spent cap drops the button entirely rather than invite a
             click guaranteed to fail until tomorrow. -->
        <OriButton
            v-if="!exhausted"
            class="guess__action"
            :label="actionText"
            variant="outline"
            color="surface"
            radius="md"
            :loading="pending"
            :disabled="pending || !canRetry"
            @click="emit('again')"
        />
    </OriSurface>
</template>

<style scoped>
/* The shell's overlay layer is pointer-events:none; this opts back in so the
   card is interactive wherever it's mounted. Narrow on purpose — one sentence
   and a button, not a screen. */
.guess {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_sm, 0.25rem);

    width: min(22rem, calc(100vw - 2rem));
    max-height: min(70dvh, 24rem);
    padding: var(--ori-size-gap_lg, 0.75rem);
    overflow-y: auto;

    pointer-events: auto;
}

.guess__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--ori-size-gap_sm, 0.25rem);
}

.guess__title {
    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-weight: 600;
}

.guess__body {
    display: flex;
    flex-direction: column;
    gap: var(--ori-size-gap_xs, 0.125rem);
}

.guess__lead {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_md, 1rem);
    line-height: 1.4;
}

/* Weight, not brand colour, carries the emphasis: --ori-color-primary is only
   2.85:1 on this surface — fine for a chunky heading, not for run-of-text
   size. A model's label can also be one long unbroken token. */
.guess__label {
    font-weight: 800;
    overflow-wrap: anywhere;
}

.guess__quiet {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    line-height: 1.4;
    /* 0.7 keeps the muted line past WCAG AA on the surface. */
    opacity: 0.7;
}

.guess__alts {
    margin: 0.15rem 0 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    line-height: 1.4;
    overflow-wrap: anywhere;

    opacity: 0.7;
}

.guess__msg {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    line-height: 1.45;
    overflow-wrap: anywhere;
}

/* The column flex's default `stretch` would recreate the full-bleed bar
   `fluid` was dropped to avoid, so the action sizes to its own text. */
.guess__action {
    align-self: flex-start;
    margin-top: var(--ori-size-gap_xs, 0.125rem);
}
</style>
