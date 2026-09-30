/**
 * The sign-in gate and the error reporting every /draw action shares: save, load,
 * assist and guess all need a session and all fail the same ways.
 */
import { useToast } from '@oriui/vue'
import { isAuthError, toApiError, useAuthGate } from '@core'

/** Toast durations, ms. */
export const TOAST = { success: 3500, info: 5000, error: 8000 } as const

export function useGatedActions(options: { onSessionExpired: () => void }) {
    const gate = useAuthGate()
    const toaster = useToast()

    // Covers the gap before a mutation turns busy, while the gate awaits the cookie
    // restore: a second trigger there would queue a second waiter, and one sign-in would
    // run the action twice.
    let awaitingGate = false

    /** Resolves true once a session exists, raising the sign-in modal if needed. */
    async function gated(reason: string): Promise<boolean> {
        if (awaitingGate) return false
        awaitingGate = true
        try {
            return await gate.ensure(reason)
        } finally {
            awaitingGate = false
        }
    }

    function reportError(err: unknown, action: string) {
        if (isAuthError(err)) {
            // Don't replay the action: the visitor may sign in as someone else.
            options.onSessionExpired()
            gated(`Your session expired — sign in to ${action}.`)
            return
        }
        const api = toApiError(err)
        toaster.error({
            text: api ? `Could not ${action}: ${api.message}` : `Could not ${action} (is the server running?).`,
            duration: TOAST.error
        })
    }

    return { gate, toaster, gated, reportError }
}
