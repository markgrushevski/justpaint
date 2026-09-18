import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useSessionStore } from './useSessionStore.ts'

/**
 * The one place the app says "this needs a signed-in visitor" (rationale in
 * docs/DECISIONS.md, 2026-09-18). The store holds only the intent;
 * `components/auth/AuthDialog.vue`, mounted once at the app root, renders it and
 * settles the promise. Nothing else may open that dialog — a local `ref` would
 * strand the caller waiting behind it.
 */
export const useAuthGate = defineStore('authGate', () => {
    const session = useSessionStore()

    const open = ref(false)
    /** Why we are asking, shown in the form ("Sign in to save your drawing."). */
    const hint = ref<string | undefined>(undefined)

    /** Everyone waiting on the modal that is currently open. */
    let waiting: ((signedIn: boolean) => void)[] = []

    /**
     * Resolves `true` once there is a session — immediately, or after the visitor
     * signs in — and `false` if they dismiss the modal. Guard an action with it:
     * `if (!(await gate.ensure('Sign in to save.'))) return`.
     *
     * A 401 needs no separate verb: the transport already forgot the dead
     * session by the time the error reaches a caller (`setUnauthorizedHandler`),
     * so `isLoggedIn` is false here and this asks for a new one.
     */
    async function ensure(reason?: string): Promise<boolean> {
        // Never judge someone anonymous while their cookie is still being
        // exchanged for a session at app start.
        await session.ready()
        if (session.isLoggedIn) return true
        // A second caller JOINS the open dialog rather than stacking another one
        // — and must not repaint the reason out from under the first.
        if (!open.value) hint.value = reason
        open.value = true
        return new Promise<boolean>((resolve) => waiting.push(resolve))
    }

    /** Called only by the dialog: authenticated (`true`) or dismissed (`false`). */
    function settle(signedIn: boolean): void {
        open.value = false
        hint.value = undefined
        const pending = waiting
        waiting = []
        for (const resolve of pending) resolve(signedIn)
    }

    return { open, hint, ensure, settle }
})
