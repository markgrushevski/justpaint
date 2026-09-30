import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { auth, isAuthError, type User } from '../api'

/**
 * Cookie-session auth against the Go backend (`jp_session`, HttpOnly). Session
 * state lives here; the fetch client (`../api/drawings`) stays store-free so
 * there is no api⇄store import cycle.
 */
export const useSessionStore = defineStore('session', () => {
    const user = ref<User | null>(null)

    const isLoggedIn = computed(() => user.value !== null)

    /**
     * Restore a session from the cookie once, when the store is constructed.
     * A 401 is the expected anonymous case; any other failure (500 / network) is
     * logged, then also falls back to anonymous — it must not hide a real error.
     */
    const restored = auth
        .me()
        .then((profile) => {
            user.value = profile
        })
        .catch((err) => {
            if (!isAuthError(err)) console.warn('Session check failed:', err)
            user.value = null
        })

    /**
     * Await that restore before concluding someone is anonymous. Nothing in the
     * app wants a re-restore: `login`/`register`/`clear` already write the
     * authoritative answer, so this resolves once and stays resolved.
     */
    async function ready(): Promise<void> {
        return restored
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
     * Forget the session without calling the server — for a 401 on a request
     * that looked authorized (the cookie expired while the tab stayed open).
     * Leaving `user` set here would show the side menu a profile nobody is
     * signed into.
     */
    function clear(): void {
        user.value = null
    }

    return { user, isLoggedIn, ready, login, register, logout, clear }
})
