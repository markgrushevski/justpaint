<script lang="ts" setup>
/**
 * RoundTimerBar — the duel round countdown: a full-width thin progress rail
 * pinned to the very top edge of the viewport, plus a compact mm:ss readout
 * just beneath it. The rail drains green → orange → red as time runs out and
 * pulses in an alert state under ten seconds.
 *
 * Presentational only: PlayView owns the clock and ticks `remaining` down;
 * this component just renders. It is `position: fixed` (escapes the shell's
 * padded top-center region to touch the very top edge) and
 * `pointer-events: none`, so it never intercepts drawing on the canvas below.
 */
import { computed } from 'vue'

const props = defineProps<{
    /** Seconds left in the round (clamped to >= 0 for display). */
    remaining: number
    /** Total round length in seconds — the rail's full extent. */
    total: number
}>()

/** 0..1 of the round still remaining — drives the rail width. */
const fraction = computed(() => (props.total > 0 ? Math.max(0, Math.min(1, props.remaining / props.total)) : 0))

/** green (plenty of time) → orange (running low) → red (final stretch / <=10s). */
const severity = computed<'ok' | 'warn' | 'danger'>(() => {
    if (props.remaining <= 10) return 'danger'
    if (fraction.value <= 0.4) return 'warn'
    return 'ok'
})

/** The last ten seconds pulse to grab attention. */
const urgent = computed(() => props.remaining > 0 && props.remaining <= 10)

/** mm:ss, rounded up so the readout never shows 0:00 while time still remains. */
const clock = computed(() => {
    const s = Math.max(0, Math.ceil(props.remaining))
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
})
</script>

<template>
    <div
        class="timer"
        :class="[`timer--${severity}`, { 'timer--urgent': urgent }]"
        role="timer"
        :aria-label="`${clock} remaining`"
    >
        <div class="timer__rail">
            <div class="timer__fill" :style="{ transform: `scaleX(${fraction})` }"></div>
        </div>
        <span class="timer__clock jp-float">{{ clock }}</span>
    </div>
</template>

<style scoped>
/* Fixed to the very top edge of the viewport — the shell's top-center region is
   padded down 0.5rem, so a fixed bar is the only way to touch the true edge.
   pointer-events:none keeps drawing live beneath it. The shell root has no
   transform, so this anchors to (and is not clipped by) the viewport. */
.timer {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    z-index: 30;

    pointer-events: none;
}

.timer__rail {
    width: 100%;
    height: 4px;
    overflow: hidden;

    background-color: color-mix(in srgb, var(--ori-color-on-surface) 12%, transparent);
}

.timer__fill {
    width: 100%;
    height: 100%;

    background-color: var(--timer-color);
    transform-origin: left center;
    /* ~1s so each tick glides instead of stepping; colour eases on threshold flips. */
    transition:
        transform 0.9s linear,
        background-color 0.4s ease;
}

/* Small centred readout tucked just under the rail. jp-float gives it the shared
   island chrome; the ink tracks the current severity colour. */
.timer__clock {
    position: absolute;
    top: 0.5rem;
    left: 50%;
    transform: translateX(-50%);

    padding: 0.05rem 0.55rem;

    color: var(--timer-color);

    font-size: var(--ori-font-size_sm, 0.85rem);
    font-weight: 800;
    font-variant-numeric: tabular-nums;
    letter-spacing: 0.02em;
}

.timer--ok {
    --timer-color: var(--ori-color-success);
}

.timer--warn {
    --timer-color: var(--ori-color-warn);
}

.timer--danger {
    --timer-color: var(--ori-color-danger);
}

.timer--urgent .timer__rail,
.timer--urgent .timer__clock {
    animation: timer-pulse 1s ease-in-out infinite;
}

@keyframes timer-pulse {
    50% {
        opacity: 0.4;
    }
}

@media (prefers-reduced-motion: reduce) {
    .timer__fill {
        transition: background-color 0.4s ease;
    }

    .timer--urgent .timer__rail,
    .timer--urgent .timer__clock {
        animation: none;
    }
}
</style>
