/**
 * The round countdown, anchored on the server clock: every response re-anchors the skew
 * and the deadline (docs/NOTES.md "The round countdown is anchored on the server clock").
 */
import { ref } from 'vue'

export interface RoundClockOptions {
    /** How long before the deadline onDeadline fires. */
    marginMs: number
    /** Whether the deadline still matters; checked every tick. */
    armed: () => boolean
    /** Fires once, `marginMs` before the deadline, and stops the clock. */
    onDeadline: () => void
}

export function useRoundClock(options: RoundClockOptions) {
    // Epoch ms, null while there is no deadline yet.
    const deadlineMs = ref<number | null>(null)
    // Captured at the first deadline; drives only the progress bar.
    const totalSeconds = ref(0)
    const remaining = ref(0)
    let offsetMs = 0
    let tick: number | null = null

    function msLeft(deadline: number): number {
        return deadline - (Date.now() + offsetMs)
    }

    function update(): void {
        remaining.value = deadlineMs.value === null ? 0 : Math.max(0, msLeft(deadlineMs.value) / 1000)
    }

    /** Re-anchor on a response; also updates the display, so it never lags a second. */
    function anchor(drawingDeadline: string | null, serverTime: string): void {
        offsetMs = Date.parse(serverTime) - Date.now()
        const next = drawingDeadline !== null ? Date.parse(drawingDeadline) : null
        if (next !== null && totalSeconds.value === 0) totalSeconds.value = Math.max(0, msLeft(next) / 1000)
        deadlineMs.value = next
        update()
    }

    function stop(): void {
        if (tick !== null) {
            clearInterval(tick)
            tick = null
        }
    }

    function start(): void {
        stop()
        tick = window.setInterval(() => {
            update()
            if (!options.armed() || deadlineMs.value === null) return
            if (msLeft(deadlineMs.value) <= options.marginMs) {
                stop()
                options.onDeadline()
            }
        }, 1000)
    }

    function reset(): void {
        stop()
        deadlineMs.value = null
        offsetMs = 0
        totalSeconds.value = 0
        remaining.value = 0
    }

    return { deadlineMs, totalSeconds, remaining, anchor, start, stop, reset }
}
