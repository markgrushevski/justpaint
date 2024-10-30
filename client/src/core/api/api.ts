import axios from 'axios'
import type { Login, Art, Register } from './types.ts'

const API = axios.create({
    baseURL: import.meta.env.VITE_URL_API,
    withCredentials: true
})

API.interceptors.request.use((req) => {
    const token = localStorage.getItem('accessToken')
    req.headers['Authorization'] = `Bearer ${token}`
    return req
})

export const mainAPI = {
    auth: {
        register: async (registerData: Register): Promise<boolean> => {
            return API.post('/auth/register', registerData).then((response) => response?.data)
        },
        login: async (loginData: Login): Promise<boolean> => {
            return API.post('/auth/login', loginData).then((response) => response?.data)
        },
        logout: async (): Promise<boolean> => {
            return API.post('/auth/logout').then((response) => response?.data)
        }
    },
    users: {
        getNickname: async (): Promise<string> => {
            return API.get('/users/nickname').then((response) => response?.data)
        }
    },
    arts: {
        getArts: async (): Promise<Art[]> => {
            return API.get('/arts').then((response) => response?.data)
        },
        saveArt: async (art: Art) => {
            return API.post('/arts', art).then((response) => response?.data)
        }
    }
}
