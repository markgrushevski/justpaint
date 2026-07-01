import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { auth, isAuthError, type User } from '../api'

/**
 * Cookie-session auth against the Go backend (`jp_session`, HttpOnly). Session
 * STATE lives here; the fetch client (`../api/drawings`) stays store-free so
 * there is no api⇄store import cycle. Separate from the legacy `useUserStore`
 * (which drives the throwaway `/legacy` app against the old backend).
 */
export const useSessionStore = defineStore('session', () => {
    const user = ref<User | null>(null)
    const isLoggedIn = computed(() => user.value !== null)

    /**
     * Restore a session from the cookie on load. A 401 is the expected anonymous
     * case; any other failure (500 / network) is logged — we still fall back to
     * anonymous, but must not silently hide a real error.
     */
    async function fetchMe(): Promise<void> {
        try {
            user.value = await auth.me()
        } catch (err) {
            if (!isAuthError(err)) console.warn('session check failed:', err)
            user.value = null
        }
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
            user.value = null
        }
    }

    return { user, isLoggedIn, fetchMe, login, register, logout }
})
