import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { auth, type User } from '../api'

/**
 * Cookie-session auth against the Go backend (`jp_session`, HttpOnly). Session
 * STATE lives here; the axios client (`../api/drawings`) stays store-free so
 * there is no api⇄store import cycle. Separate from the legacy `useUserStore`
 * (which drives the throwaway `/legacy` app against the old backend).
 */
export const useSessionStore = defineStore('session', () => {
    const user = ref<User | null>(null)
    const loading = ref(false)
    const isLoggedIn = computed(() => user.value !== null)

    /** Restore a session from the cookie on load; any failure ⇒ anonymous. */
    async function fetchMe(): Promise<void> {
        loading.value = true
        try {
            user.value = await auth.me()
        } catch {
            user.value = null
        } finally {
            loading.value = false
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

    return { user, loading, isLoggedIn, fetchMe, login, register, logout }
})
