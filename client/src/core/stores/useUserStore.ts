import { defineStore } from 'pinia'
import { ref } from 'vue'
import { mainAPI, type User, type UserLogin } from '@core'

export const useUserStore = defineStore('auth', () => {
    const user = ref<User | null>(null)

    const loginData = ref<UserLogin>({ nickname: '', password: '' })

    async function login() {
        await mainAPI.auth.login(loginData.value)
    }

    async function logout() {
        await mainAPI.auth.logout()
    }

    return { user, loginData, login, logout }
})
