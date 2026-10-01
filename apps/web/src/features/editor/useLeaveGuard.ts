/**
 * Asks before a route change drops work. `question()` returns what to ask, or null to
 * let the navigation through; bind `pending` to a ConfirmDialog and answer with
 * `leave` / `stay`. A reload or a closed tab gets the browser's own prompt instead.
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
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
        // A second navigation (browser Back) while asking replaces the first one.
        settle?.(false)
        pending.value = ask
        return new Promise<boolean>((resolve) => (settle = resolve))
    })

    function onBeforeUnload(e: BeforeUnloadEvent) {
        if (question()) e.preventDefault()
    }
    onMounted(() => window.addEventListener('beforeunload', onBeforeUnload))
    onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload))

    function answer(leave: boolean) {
        pending.value = null
        settle?.(leave)
        settle = null
    }

    return { pending, leave: () => answer(true), stay: () => answer(false) }
}
