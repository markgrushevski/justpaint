/**
 * Asks before a route change drops work. `question()` returns what to ask, or null to
 * let the navigation through; bind `pending` to a ConfirmDialog and answer with
 * `leave` / `stay`.
 */
import { ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'

export interface LeaveQuestion {
    title: string
    message: string
    confirmText: string
}

export function useLeaveGuard(question: () => LeaveQuestion | null) {
    const pending = ref<LeaveQuestion | null>(null)
    let settle: ((leave: boolean) => void) | null = null

    onBeforeRouteLeave(() => {
        const ask = question()
        if (!ask) return true
        pending.value = ask
        return new Promise<boolean>((resolve) => (settle = resolve))
    })

    function answer(leave: boolean) {
        pending.value = null
        settle?.(leave)
        settle = null
    }

    return { pending, leave: () => answer(true), stay: () => answer(false) }
}
