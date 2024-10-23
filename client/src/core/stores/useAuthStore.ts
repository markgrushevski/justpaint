import { defineStore } from 'pinia'
import { ref } from 'vue'
import { API, type User, type UserLogin } from '@core'

export const useAuthStore = defineStore('auth', () => {
    const user = ref<User | null>(null)

    const loginData = ref<UserLogin>({ nickname: '', password: '' })

    async function login() {
        await API.auth.login(loginData.value)
    }

    async function logout() {
        await API.auth.logout()
    }

    return { user, loginData, login, logout }
})
