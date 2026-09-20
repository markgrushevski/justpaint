<script lang="ts">
/**
 * Confidence bands, highest first. The endpoint hands back a number in 0..1, and a
 * number is not how a person tells you how sure they are — printing "0.82" also
 * invites the reader to treat a model's self-report as a measurement. So the card
 * says it the way a person would and keeps the number to itself.
 *
 * Copy, not contract: nothing downstream reads these thresholds (the same licence
 * PracticeResult's score bands take).
 */
const BANDS: { min: number; text: string }[] = [
    { min: 0.85, text: 'I’d bet on it.' },
    { min: 0.6, text: 'Fairly sure about that one.' },
    { min: 0.35, text: 'Not too sure, though.' },
    { min: 0, text: 'Honestly, that’s a wild guess.' }
]

/**
 * What the card is showing — ONE discriminant, the precedent PracticeView's
 * `Phase` sets. It replaced a set of independent booleans whose only guarantee
 * that they stayed consistent was that every call site remembered to clear the
 * other three; the body below switches on this and can therefore never render the
 * empty card that "all of them falsy" used to produce.
 *
 * `idle` is the card open with nothing asked yet — today that is the blank-canvas
 * case, and it is the card (not a disabled button that cannot speak on touch)
 * that says so. The qualifier `exhausted` stays BESIDE this rather than folded in:
 * it is orthogonal to `error`, exactly as PracticeView keeps `submitExhausted`
 * beside its phase.
 */
export type GuessStatus = 'idle' | 'pending' | 'answered' | 'error'

/**
 * The action's label, one line per status. A `Record` over the union rather than
 * a chain of truthiness tests, so adding a status is a compile error here instead
 * of a silently blank button — and, unlike a `switch`, it needs no unreachable
 * default to satisfy `vue/return-in-computed-property`.
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
 * GuessResult — the card the AI's reading of the canvas lands in, floated in
 * /draw's overlay layer. It is the ONE surface this feature has: the wait, the
 * answer and every failure all render here, because the visitor is already
 * looking at it and a toast for any of them would be both easy to miss and (for
 * the daily cap) the wrong colour of news.
 *
 * The answer is written as a SENTENCE, not a readout. The fun of asking a model
 * what you drew is that it talks back — and that it is often wrong — so the guess
 * leads in bold, the certainty follows as a human aside, and the runner-ups sit
 * underneath as a quiet "or maybe", which is where most of the comedy lives.
 *
 * Presentational only, like PracticeResult: props in, events out, no API calls and
 * no knowledge of the endpoint — DrawView owns the request, the budget and the
 * canvas.
 */
import { computed } from 'vue'
import { OriButton, OriSurface } from '@oriui/vue'
import IconButton from './ui/IconButton.vue'

const props = withDefaults(
    defineProps<{
        /** Which of the four things this card is showing — the discriminant. */
        status?: GuessStatus
        /** The AI's best reading; read only while `status === 'answered'`. */
        label?: string | null
        /** Its certainty, 0..1 — rendered as a phrase, never as the number. */
        confidence?: number
        /** 0–2 runner-up guesses; the "or maybe" line. */
        alternatives?: string[]
        /** A failure message, in the SERVER's own words; read only on `error`. */
        error?: string
        /**
         * The failure was the spent DAILY cap rather than a fault or the per-IP
         * tier that clears in seconds. An ordinary, expected outcome — two guesses
         * a day is the whole budget — so the card says so plainly and drops the
         * retry, which could not succeed today. Qualifies `error`; never stands
         * on its own.
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
 * The action doubles as the pending indicator, exactly as the assist panel's Draw
 * button does: `loading` gives it oriui's spinner plus `aria-busy` and disables
 * it, so the several-second wait shows up on the control that caused it instead
 * of nowhere at all. Its wording comes from the exhaustive {@link ACTION_TEXT}.
 */
const actionText = computed(() => ACTION_TEXT[props.status])

/** `loading` + `disabled` both key off the wait; naming it keeps the template honest. */
const pending = computed(() => props.status === 'pending')
</script>

<template>
    <OriSurface class="guess" role="group" aria-labelledby="guess-title">
        <!-- Header + explicit close, mirroring the assist panel: the island trigger
             that opened this can be off-screen on a narrow phone, so the card is
             always dismissible from within. -->
        <div class="guess__head">
            <span id="guess-title" class="guess__title">AI guess</span>
            <IconButton icon="close" label="Dismiss the guess" placement="bottom" @click="emit('dismiss')" />
        </div>

        <!-- The answer lands seconds after the click, long after focus has moved on,
             so a screen reader has to be told it arrived (role=status is an
             implicit polite live region). -->
        <!-- One arm per `GuessStatus`, and the last is a bare v-else so the chain
             is exhaustive by construction: there is no combination of props that
             leaves this body empty. -->
        <div class="guess__body" role="status">
            <template v-if="status === 'pending'">
                <p class="guess__lead">Looking at your drawing…</p>
                <p class="guess__quiet">It gets redrawn on the server first, so this takes a few seconds.</p>
            </template>

            <template v-else-if="status === 'error'">
                <p class="guess__lead">{{ exhausted ? 'That’s your guessing for today' : 'The AI didn’t answer' }}</p>
                <!-- The server's own wording, verbatim: it is the only side that
                     knows whether the visitor spent their guesses or the service
                     spent its budget, and it says the first without leaking the
                     second. Hardcoding our own sentence here would drift from it. -->
                <p class="guess__msg">{{ error }}</p>
            </template>

            <template v-else-if="status === 'answered'">
                <p class="guess__lead">
                    I think this is <strong class="guess__label">{{ label }}</strong>
                </p>
                <p class="guess__quiet">{{ confidenceText }}</p>
                <!-- The runner-ups are the point of the feature, but they are an
                     aside to the headline guess — quiet, lower-case, one line. -->
                <p v-if="alternatives.length > 0" class="guess__alts">or maybe: {{ alternatives.join(', ') }}</p>
            </template>

            <!-- idle: nothing has been asked. The blank-canvas reason lives HERE
                 rather than in the trigger's tooltip — a disabled button takes no
                 focus and oriui's tooltip needs hover or focus-within, so on a
                 phone the trigger could only be a dimmed glyph with no stated
                 reason. The card is already the single home for every outcome. -->
            <template v-else>
                <p class="guess__lead">Draw something first</p>
                <p class="guess__quiet">Then I can tell you what I think it is.</p>
            </template>
        </div>

        <!-- Subordinate to the answer above it, so it wears the same secondary
             treatment PracticeResult gives its non-primary action and sizes to its
             own text: a full-bleed primary bar would be the loudest thing on the
             screen while spending 1 of only 2 daily calls.

             A spent cap drops it entirely (PracticeView's `submitExhausted`
             precedent): inviting a click that is guaranteed to fail until tomorrow
             is worse than saying so. Closing, and the canvas behind, stay usable. -->
        <OriButton
            v-if="!exhausted"
            class="guess__action"
            :text="actionText"
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
/* Centred in the shell's overlay layer, which is pointer-events:none so it never
   blocks drawing — the card opts back in for itself (like PracticeResult) so it
   is correct wherever it is mounted. Narrow on purpose: this is one sentence and
   a button, not a screen, and a short card is a card that clears the toolbar. */
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

/* The guess itself carries the emphasis with WEIGHT, not with the brand colour:
   --ori-color-primary is 2.85:1 on the surface (the ratio EmptyState works around
   by moving its wordmark onto the background), which is fine for a chunky heading
   and not fine for the normal-size run of text this sits in. A model's label can
   also be a single long unbroken token, which would otherwise push the card wider
   than a phone. */
.guess__label {
    font-weight: 800;
    overflow-wrap: anywhere;
}

.guess__quiet {
    margin: 0;

    color: var(--ori-color-on-surface);

    font-size: var(--ori-font-size_sm, 0.85rem);
    line-height: 1.4;
    /* 0.7 keeps the muted line past WCAG AA on the surface (the same value the
       empty state and the side menu use for their hint text). */
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

/* The card is a column flex, whose default `stretch` would re-create the
   full-bleed bar `fluid` was dropped to avoid — so the action sizes to its own
   text and sits under the start of the sentence it follows. */
.guess__action {
    align-self: flex-start;
    margin-top: var(--ori-size-gap_xs, 0.125rem);
}
</style>
