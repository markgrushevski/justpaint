/**
 * "What did I draw?" (docs/API.md §13): the card that asks the AI to name the drawing.
 * A couple of guesses a day, so every guard here avoids spending one on nothing.
 */
import { computed, ref, watch, type Ref, type ShallowRef } from 'vue'
import type { Editor } from '@justpaint/editor'
import { isAuthError, isBudgetExhausted, isRateLimited, toApiError, useGuess } from '@core'
import type { Guess } from '@core'
import type { GuessStatus } from './GuessResult.vue'

export function useGuessPanel(
    editor: ShallowRef<Editor | null>,
    isEmpty: Ref<boolean>,
    gated: (reason: string) => Promise<boolean>,
    reportError: (err: unknown, action: string) => void
) {
    // Not the mutation's `isPending`: the card outlives the request to show its answer.
    const mutation = useGuess()
    const pending = computed(() => mutation.isPending.value)
    const open = ref(false)
    /**
     * What the card shows; only `setStatus` moves it, together with the payloads below.
     * It differs from `pending` only after a mid-call dismiss: the status stays `pending`,
     * so reopening resumes the already-paid-for wait.
     */
    const status = ref<GuessStatus>('idle')
    const result = ref<Guess | null>(null)
    const error = ref('')
    const exhausted = ref(false)
    // Set when the document is replaced mid-guess, so a late answer is dropped. A plain
    // dismiss does not set it: that answer is still about the canvas on screen.
    let stale = false

    /** Move the card to `next`, clearing every payload with it. */
    function setStatus(next: GuessStatus) {
        status.value = next
        result.value = null
        error.value = ''
        exhausted.value = false
    }

    function dismiss() {
        open.value = false
        // An in-flight call keeps `pending`, so reopening lands back on the paid-for wait.
        if (!pending.value) setStatus('idle')
    }

    /** Unlike dismiss, this also disowns an in-flight call: its canvas no longer exists. */
    function invalidate() {
        stale = true
        open.value = false
        setStatus('idle')
    }

    // Undoing to a blank canvas is a document swap too, and a standing answer would share
    // the overlay slot with the hint.
    watch(isEmpty, (empty) => {
        if (empty) invalidate()
    })

    // Failures stay in the card, not a toast: with two guesses a day, running out is
    // expected. A lapsed session goes through the auth gate instead.
    function onError(err: unknown) {
        if (stale) return
        if (isAuthError(err)) {
            dismiss()
            reportError(err, 'guess your drawing')
            return
        }
        setStatus('error')
        const api = toApiError(err)
        if (isBudgetExhausted(err)) {
            exhausted.value = true
            error.value = api?.message ?? 'That is every AI guess you get today.'
            return
        }
        // Unlike the daily budget, the per-IP 429 clears in seconds (docs/API.md §3.1).
        error.value = isRateLimited(err)
            ? `${api?.message ?? 'Too many requests just now'} — try again in a moment.`
            : (api?.message ?? 'The AI could not be reached. Try again.')
    }

    async function request() {
        if (!editor.value || pending.value) return
        // A disabled button's tooltip can't reach touch or keyboard, so the card explains.
        if (isEmpty.value) {
            setStatus('idle')
            open.value = true
            return
        }
        if (!(await gated('Sign in to have the AI guess your drawing.'))) return
        // Browser Back can unmount the view while the modal is up.
        const ed = editor.value
        if (!ed) return
        stale = false
        // Open before firing, so the several-second wait has somewhere to live.
        setStatus('pending')
        open.value = true
        mutation.mutate(ed.getDocument(), {
            onSuccess: (answer) => {
                if (stale) return
                setStatus('answered')
                result.value = answer
            },
            onError
        })
    }

    // A toggle: a second click hides the card rather than paying again, and a non-idle
    // status is an unseen answer to reopen onto.
    function toggle() {
        if (open.value) {
            dismiss()
            return
        }
        if (status.value !== 'idle') {
            open.value = true
            return
        }
        request()
    }

    return { open, status, result, error, exhausted, dismiss, invalidate, request, toggle }
}
