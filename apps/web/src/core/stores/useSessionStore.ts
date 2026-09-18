import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { auth, isAuthError, type User } from '../api'

/**
 * Cookie-session auth against the Go backend (`jp_session`, HttpOnly). Session
 * STATE lives here; the fetch client (`../api/drawings`) stays store-free so
 * there is no api⇄store import cycle.
 */
export const useSessionStore = defineStore('session', () => {
    const user = ref<User | null>(null)
    const isLoggedIn = computed(() => user.value !== null)

    /** The in-flight cookie restore, shared by every concurrent caller. */
    let restoring: Promise<void> | null = null
    /** Whether a restore has completed at least once (success OR anonymous). */
    let restored = false

    /**
     * Restore a session from the cookie on load. A 401 is the expected anonymous
     * case; any other failure (500 / network) is logged — we still fall back to
     * anonymous, but must not silently hide a real error.
     *
     * Concurrent callers share one request: three views call this on mount and
     * the auth gate awaits it, and a burst of /me calls all answering the same
     * cookie is pure waste.
     */
    function fetchMe(): Promise<void> {
        if (restoring) return restoring
        const done = (async () => {
            try {
                user.value = await auth.me()
            } catch (err) {
                if (!isAuthError(err)) console.warn('session check failed:', err)
                user.value = null
            } finally {
                restoring = null
                restored = true
            }
        })()
        restoring = done
        return done
    }

    /**
     * Await the initial cookie restore before concluding that someone is
     * anonymous. The views fire `fetchMe()` on mount WITHOUT awaiting it, so a
     * click landing inside that window reads `isLoggedIn === false` for a
     * visitor who is in fact signed in — which merely cost a stray 401 before,
     * and would now put a sign-in modal in their face. Restores at most once.
     */
    function ready(): Promise<void> {
        return restored ? Promise.resolve() : fetchMe()
    }

    async function login(loginId: string, password: string): Promise<void> {
        user.value = await auth.login({ login: loginId, password })
    }

    async function register(loginId: string, password: string): Promise<void> {
        user.value = await auth.register({ login: loginId, password })
    }

    async function logout(): Promise<void> {
        try {
            await auth.logout()
        } finally {
            clear()
        }
    }

    /**
     * Forget the session WITHOUT calling the server — for when the server has
     * already told us it is gone (a 401 on a request we thought was authorized:
     * the cookie expired while the tab stayed open). Leaving `user` set there is
     * what used to leave the side menu showing a profile nobody was signed into.
     */
    function clear(): void {
        user.value = null
    }

    return { user, isLoggedIn, fetchMe, ready, login, register, logout, clear }
})
