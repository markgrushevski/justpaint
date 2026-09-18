import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useSessionStore } from './useSessionStore.ts'

/**
 * The one place the app says "this needs a signed-in visitor".
 *
 * Before this gate every caller improvised: /draw opened the side drawer and
 * toasted "Sign in from the menu…" — at a menu that was still rendering the
 * stale profile, because nothing dropped the expired session — while /play fell
 * into a terminal error phase. Now an action awaits `ensure()`: already signed
 * in resolves `true` at once, otherwise ONE modal opens and the action resumes
 * exactly where it stopped, or gives up if the visitor dismisses it.
 *
 * The store holds only the intent; `components/auth/AuthDialog.vue` (mounted
 * once at the app root) renders it and settles the promise. Nothing else may
 * open that dialog — a local `ref` would lose the caller waiting behind it.
 */
export const useAuthGate = defineStore('authGate', () => {
    const session = useSessionStore()

    const open = ref(false)
    /** Why we are asking, shown in the form ("Sign in to save your drawing."). */
    const hint = ref<string | undefined>(undefined)

    /** Everyone waiting on the modal that is currently open. A second `ensure()`
     *  joins this queue instead of stacking a second dialog over the first. */
    let waiting: ((signedIn: boolean) => void)[] = []

    /**
     * Resolves `true` once there is a session — immediately, or after the
     * visitor signs in — and `false` if they dismiss the modal instead. Guard an
     * action with it: `if (!(await gate.ensure('Sign in to save.'))) return`.
     */
    async function ensure(reason?: string): Promise<boolean> {
        // Never judge someone anonymous while their cookie is still being
        // exchanged for a session (the views restore it without awaiting it).
        await session.ready()
        if (session.isLoggedIn) return true
        if (reason) hint.value = reason
        open.value = true
        return new Promise<boolean>((resolve) => waiting.push(resolve))
    }

    /**
     * A request just came back 401: the cookie expired or was cleared server
     * side while the tab stayed open. Drop the stale user FIRST — otherwise
     * `isLoggedIn` is still true and `ensure()` would cheerfully resolve `true`
     * against a session that no longer exists — then ask for a new one.
     */
    function recover(reason?: string): Promise<boolean> {
        session.clear()
        return ensure(reason)
    }

    /** Called only by the dialog: authenticated (`true`) or dismissed (`false`). */
    function settle(signedIn: boolean): void {
        open.value = false
        hint.value = undefined
        const pending = waiting
        waiting = []
        for (const resolve of pending) resolve(signedIn)
    }

    return { open, hint, ensure, recover, settle }
})
