import { defineStore } from 'pinia'
import { computed, reactive, ref } from 'vue'
import { type Login, mainAPI } from '../api'

export const useUserStore = defineStore('auth', () => {
    const isLoggedIn = ref(false)

    const formData = ref<Login>({
        nickname: '',
        password: ''
    })

    function $reset() {
        isLoggedIn.value = false
        formData.value = {
            nickname: '',
            password: ''
        }
    }

    return { isLoggedIn, formData, $reset }
})
