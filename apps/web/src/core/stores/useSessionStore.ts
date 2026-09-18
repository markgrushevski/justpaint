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

    /**
     * Restore a session from the cookie, ONCE, when the store is constructed.
     * A 401 is the expected anonymous case; any other failure (500 / network) is
     * logged — we still fall back to anonymous, but must not silently hide a
     * real error.
     */
    const restored = (async () => {
        try {
            user.value = await auth.me()
        } catch (err) {
            if (!isAuthError(err)) console.warn('session check failed:', err)
            user.value = null
        }
    })()

    /**
     * Await that restore before concluding someone is anonymous. Nothing in the
     * app wants a RE-restore: `login`/`register`/`clear` already write the
     * authoritative answer, so this resolves once and stays resolved.
     */
    function ready(): Promise<void> {
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
     * Forget the session WITHOUT calling the server — for when the server has
     * already told us it is gone (a 401 on a request we thought was authorized:
     * the cookie expired while the tab stayed open). Leaving `user` set there is
     * what used to leave the side menu showing a profile nobody was signed into.
     */
    function clear(): void {
        user.value = null
    }

    return { user, isLoggedIn, ready, login, register, logout, clear }
})
